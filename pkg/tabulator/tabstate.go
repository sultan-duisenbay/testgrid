/*
Copyright 2022 The TestGrid Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package tabulator processes test group state into tab state.
package tabulator

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"path"
	"sync"
	"time"

	"bitbucket.org/creachadair/stringset"
	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/config/snapshot"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	statepb "github.com/GoogleCloudPlatform/testgrid/pb/state"
	"github.com/GoogleCloudPlatform/testgrid/pkg/updater"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
	"github.com/GoogleCloudPlatform/testgrid/util/metrics"
)

const componentName = "tabulator"

// Metrics holds metrics relevant to this controller.
type Metrics struct {
	UpdateState  metrics.Cyclic
	DelaySeconds metrics.Duration
}

// CreateMetrics creates metrics for this controller
func CreateMetrics(factory metrics.Factory) *Metrics {
	return &Metrics{
		UpdateState:  factory.NewCyclic(componentName),
		DelaySeconds: factory.NewDuration("delay", "Seconds tabulator is behind schedule", "component"),
	}
}

// Fixer should adjust the dashboard queue until the context expires.
type Fixer func(context.Context, *config.DashboardQueue) error

// Update tab state with the given frequency continuously. If freq == 0, runs only once.
//
// Copies the grid into the tab state. If filter is set, will remove unneeded data.
// Runs on each dashboard in allowedDashboards, or all of them in the config if not specified
func Update(ctx context.Context, client gcs.ConditionalClient, mets *Metrics, configPath gcs.Path, concurrency int, gridPathPrefix, tabsPathPrefix string, allowedDashboards []string, confirm, filter bool, freq time.Duration, fixers ...Fixer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if concurrency < 1 {
		return fmt.Errorf("concurrency must be positive, got: %d", concurrency)
	}
	log := logrus.WithField("config", configPath)

	var q config.DashboardQueue

	log.Debug("Observing config...")
	cfgChanged, err := snapshot.Observe(ctx, log, client, configPath, time.NewTicker(time.Minute).C)
	if err != nil {
		return fmt.Errorf("error while observing config %q: %w", configPath.String(), err)
	}

	var cfg *snapshot.Config
	fixSnapshot := func(newConfig *snapshot.Config) {
		cfg = newConfig
		if len(allowedDashboards) != 0 {
			dashes := make([]*configpb.Dashboard, 0, len(allowedDashboards))
			for _, d := range allowedDashboards {
				dash, ok := cfg.Dashboards[d]
				if !ok {
					log.Errorf("Could not find requested dashboard %q in config", d)
					continue
				}
				dashes = append(dashes, dash)
			}
			q.Init(log, dashes, time.Now())
			return
		}
		dashes := make([]*configpb.Dashboard, 0, len(cfg.Dashboards))
		for _, dash := range cfg.Dashboards {
			dashes = append(dashes, dash)
		}

		q.Init(log, dashes, time.Now())
	}

	fixSnapshot(<-cfgChanged)

	go func(ctx context.Context) {
		fixCtx, fixCancel := context.WithCancel(ctx)
		var fixWg sync.WaitGroup
		fixAll := func() {
			n := len(fixers)
			log.WithField("fixers", n).Debug("Starting fixers on current dashboards...")
			fixWg.Add(n)
			for i, fix := range fixers {
				go func(i int, fix Fixer) {
					defer fixWg.Done()
					if err := fix(fixCtx, &q); err != nil && !errors.Is(err, context.Canceled) {
						log.WithError(err).WithField("fixer", i).Warning("Fixer failed")
					}
				}(i, fix)
			}
			log.WithField("fixers", n).Info("Started fixers on current dashboards.")
		}

		ticker := time.NewTicker(time.Minute)
		fixAll()
		defer ticker.Stop()
		for {
			depth, next, when := q.Status()
			log := log.WithField("depth", depth)
			if next != nil {
				log = log.WithField("next", &next)
			}
			delay := time.Since(when)
			if delay < 0 {
				delay = 0
				log = log.WithField("sleep", -delay)
			}
			mets.DelaySeconds.Set(delay, componentName)
			log.Debug("Calculated metrics")

			select {
			case <-ctx.Done():
				ticker.Stop()
				fixCancel()
				fixWg.Wait()
				return
			case newConfig := <-cfgChanged:
				log.Info("Configuration changed")
				fixCancel()
				fixWg.Wait()
				fixCtx, fixCancel = context.WithCancel(ctx)
				fixSnapshot(newConfig)
				fixAll()
			case <-ticker.C:
			}
		}
	}(ctx)

	// Set up threads
	var active stringset.Set
	var waiting stringset.Set
	var lock sync.Mutex

	dashboardNames := make(chan string)

	update := func(log *logrus.Entry, dashName string) error {
		dashboard, ok := cfg.Dashboards[dashName]
		if !ok {
			return fmt.Errorf("no dashboard named %q", dashName)
		}

		for _, tab := range dashboard.GetDashboardTab() {
			log := log.WithField("tab", tab.GetName())
			fromPath, err := updater.TestGroupPath(configPath, gridPathPrefix, tab.TestGroupName)
			if err != nil {
				return fmt.Errorf("can't make tg path %q: %w", tab.TestGroupName, err)
			}
			toPath, err := TabStatePath(configPath, tabsPathPrefix, dashName, tab.Name)
			if err != nil {
				return fmt.Errorf("can't make dashtab path %s/%s: %w", dashName, tab.Name, err)
			}
			log.WithFields(logrus.Fields{
				"from": fromPath.String(),
				"to":   toPath.String(),
			}).Info("Calculating state")
			if !filter && confirm {
				// copy-only mode
				_, err = client.Copy(ctx, *fromPath, *toPath)
				if err != nil {
					if errors.Is(err, storage.ErrObjectNotExist) {
						log.WithError(err).Info("Original state does not exist.")
					} else {
						return fmt.Errorf("can't copy from %q to %q: %w", fromPath.String(), toPath.String(), err)
					}
				}
			}
			if filter {
				err := tabulate(ctx, client, tab, *fromPath, *toPath, confirm)
				if err != nil {
					if errors.Is(errors.Unwrap(err), storage.ErrObjectNotExist) {
						log.WithError(err).Info("Original state does not exist")
					} else {
						return fmt.Errorf("can't calculate state: %w", err)
					}
				}
			}
		}
		return nil
	}

	// Run threads continuously
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			for dashName := range dashboardNames {
				lock.Lock()
				start := active.Add(dashName)
				if !start {
					waiting.Add(dashName)
				}
				lock.Unlock()
				if !start {
					continue
				}

				log := log.WithField("dashboard", dashName)
				finish := mets.UpdateState.Start()

				if err := update(log, dashName); err != nil {
					finish.Fail()
					q.Fix(dashName, time.Now().Add(freq/2), false)
					log.WithError(err).Error("Failed to generate tab state")
				} else {
					finish.Success()
					log.Info("Built tab state")
				}

				lock.Lock()
				active.Discard(dashName)
				restart := waiting.Discard(dashName)
				lock.Unlock()
				if restart {
					q.Fix(dashName, time.Now(), false)
				}
			}
		}()
	}
	defer wg.Wait()
	defer close(dashboardNames)

	return q.Send(ctx, dashboardNames, freq)
}

// TabStatePath returns the path for a given tab.
func TabStatePath(configPath gcs.Path, tabPrefix, dashboardName, tabName string) (*gcs.Path, error) {
	name := path.Join(tabPrefix, dashboardName, tabName)
	u, err := url.Parse(name)
	if err != nil {
		return nil, fmt.Errorf("invalid url %s: %w", name, err)
	}
	np, err := configPath.ResolveReference(u)
	if err != nil {
		return nil, fmt.Errorf("resolve reference: %w", err)
	}
	if np.Bucket() != configPath.Bucket() {
		return nil, fmt.Errorf("tabState %s should not change bucket", name)
	}
	return np, nil
}

func tabulate(ctx context.Context, client gcs.Client, cfg *configpb.DashboardTab, testGroupPath, tabStatePath gcs.Path, confirm bool) error {
	r, _, err := client.Open(ctx, testGroupPath)
	if err != nil {
		return fmt.Errorf("client.Open(%s): %w", testGroupPath.String(), err)
	}
	defer r.Close()
	z, err := zlib.NewReader(r)
	if err != nil {
		return fmt.Errorf("zlib.NewReader: %w", err)
	}
	defer z.Close()
	buf, err := ioutil.ReadAll(z)
	if err != nil {
		return fmt.Errorf("ioutil.ReadAll: %w", err)
	}
	var g statepb.Grid
	if err = proto.Unmarshal(buf, &g); err != nil {
		return fmt.Errorf("proto.Unmarshal: %w", err)
	}

	newRows, err := filterGrid(cfg.GetBaseOptions(), g.GetRows())
	if err != nil {
		return fmt.Errorf("filterGrid: %w", err)
	}
	g.Rows = newRows

	if confirm {
		buf, err = proto.Marshal(&g)
		if err != nil {
			return fmt.Errorf("proto.Marshal: %w", err)
		}

		var zbuf bytes.Buffer
		zw := zlib.NewWriter(&zbuf)
		_, err = zw.Write(buf)
		if err != nil {
			return fmt.Errorf("zlib.Write: %w", err)
		}
		zw.Close()

		_, err = client.Upload(ctx, tabStatePath, zbuf.Bytes(), false, "")
		if err != nil {
			return fmt.Errorf("client.Upload(%s): %w", tabStatePath.String(), err)
		}
	}
	return nil
}

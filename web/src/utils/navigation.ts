/**
 * Navigates application to a specified dashboard.
 * @fires location-changed
 * @param {string} path
 */
export function navigateDashboard(name: string){
  const url = new URL(location.href);
  url.pathname = `dashboards/${name}`;
  history.pushState(null, '', url);
  window.dispatchEvent(new CustomEvent('location-changed'));
}

/**
 * Navigates application to a specified group.
 * @fires location-changed
 * @param {string} path
 */
export function navigateGroup(name: string){
  const url = new URL(location.href);
  url.pathname = `groups/${name}`;
  history.pushState(null, '', url);
  window.dispatchEvent(new CustomEvent('location-changed'));
}

/**
 * Changes the pathname (for tab) without reloading
 * @param {string} dashboard
 * @param {string} tab
 */
export function navigateTabWithoutReload(dashboard: string, tab: string){
  const url = new URL(location.href)
  if (tab === 'Summary' || tab === undefined){
    url.pathname = `dashboards/${dashboard}`
  } else {
    url.pathname = `dashboards/${dashboard}/tabs/${tab}`
  }
  history.pushState(null, '', url);
}

/**
 * Changes the pathname (for tab) without reloading
 * @param {string} group
 * @param {string} dashboard
 */
export function navigateDashboardWithoutReload(group: string, dashboard: string){
  const url = new URL(location.href)
  if (dashboard === group || dashboard === undefined){
    url.pathname = `groups/${group}`
  } else {
    url.pathname = `groups/${group}/dashboards/${dashboard}`
  }
  history.pushState(null, '', url);
}

import { LitElement, html, css } from 'lit';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { customElement, property, state } from 'lit/decorators.js';
import { map } from 'lit/directives/map.js';
import { when } from 'lit/directives/when.js';
import { navigateDashboardWithoutReload } from './utils/navigation.js';
import { GetDashboardGroupResponse } from './gen/pb/api/v1/data.js';
import '@material/mwc-tab';
import '@material/mwc-tab-bar';
import '@material/web/divider/divider.js';
import './testgrid-group-summary';
import './testgrid-dashboard-grid.js';

/**
 * Class definition for the `testgrid-groupd-dashboard` element.
 * Acts as a container for group summary or dashboard-grid element.
 */
@customElement('testgrid-group-dashboard')
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export class TestgridGroupDashboard extends LitElement {

  @state()
  dashboardNames: string[] = [];

  @state()
  activeIndex = 0;

  @property({ type: Boolean })
  showDashboard = false;

  @property({ type: String })
  groupName = '';

  @property({ type: String })
  dashboardName?: string;

  // set the functionality when any tab is clicked on
  private onDashboardActivated(event: CustomEvent<{index: number}>) {
    const dashboardIndex = event.detail.index;
    if (dashboardIndex === this.activeIndex){
      return
    }

    this.dashboardName = this.dashboardNames[dashboardIndex];
    if (this.activeIndex === 0 || dashboardIndex === 0){
      this.showDashboard = !this.showDashboard;
    }
    this.activeIndex = dashboardIndex;
    navigateDashboardWithoutReload(this.groupName, this.dashboardName)
  }

  /**
   * Lit-element lifecycle method.
   * Invoked when a component is added to the document's DOM.
   */
  connectedCallback() {
    super.connectedCallback();
    this.fetchDashboardNames();
  }

  /**
   * Lit-element lifecycle method.
   * Invoked on each update to perform rendering tasks.
   */
  render() {
    var tabBar = html`${
      // make sure we only render the tabs when there are tabs
      when(this.dashboardNames.length > 0, () => html`
        <mwc-tab-bar .activeIndex=${this.activeIndex} @MDCTabBar:activated="${this.onDashboardActivated}">
          ${map(
            this.dashboardNames,(name: string) => html`<mwc-tab label=${name}></mwc-tab>`
          )}
        </mwc-tab-bar>`)
    }`;
    return html`
      ${tabBar}
      ${html`<md-divider></md-divider>`}
      ${!this.showDashboard ?
        html`<testgrid-group-summary .groupName=${this.groupName}></testgrid-group-summary>` :
        html`<testgrid-dashboard-grid .dashboardName=${this.dashboardName} ?showTab=${false}></testgrid-dashboard-grid>`}
    `;
  }

  // fetch the tab names to populate the tab bar
  private async fetchDashboardNames() {
    try {
      const response = await fetch(
        `http://${process.env.API_HOST}:${process.env.API_PORT}/api/v1/dashboard-groups/${this.groupName}`
      );
      if (!response.ok) {
        throw new Error(`HTTP error: ${response.status}`);
      }
      const data = GetDashboardGroupResponse.fromJson(await response.json());
      var dashboardNames: string[] = [`${this.groupName}`];
      data.dashboards.forEach(dashboard => {
        dashboardNames.push(dashboard.name);
      });
      this.dashboardNames = dashboardNames;
      this.highlightIndex(this.dashboardName);
    } catch (error) {
      console.error(`Could not get dashboards for the group: ${error}`);
    }
  }

  // identify which tab to highlight on the tab bar
  private highlightIndex(dashboardName: string | undefined) {
    if (dashboardName === undefined){
      this.activeIndex = 0;
      return
    }
    var index = this.dashboardNames.indexOf(dashboardName);
    if (index > -1){
      this.activeIndex = index;
    }
  }

  static styles = css`
    mwc-tab{
      --mdc-typography-button-letter-spacing: 0;
      --mdc-tab-horizontal-padding: 12px;
      --mdc-typography-button-font-size: 0.8rem;
      --mdc-theme-primary: #4B607C;
    }

    md-divider{
      --md-divider-thickness: 2px;
    }
`;
}

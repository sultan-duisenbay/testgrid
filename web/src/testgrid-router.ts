import { LitElement, html } from "lit";
import { customElement } from "lit/decorators.js";
import { Router } from "@lit-labs/router";
import './testgrid-group-dashboard';
import './testgrid-dashboard-grid';
import './testgrid-index';

// Defines the type of params used for rendering components under different paths
interface RouteParameter {
    [key: string]: string | undefined;
}

/**
 * Class definition for the `testgrid-router` element.
 * Handles the routing logic.
 */
@customElement('testgrid-router')
export class TestgridRouter extends LitElement{
  private router = new Router(this, [
    {
      path: '/groups/:group/*', 
      render: (params: RouteParameter) => html`<testgrid-group-dashboard .groupName=${params.group} .dashboardName=${params[0]} showDashboard></testgrid-group-dashboard>`,
    },
    {
      path: '/groups/:group', 
      render: (params: RouteParameter) => html`<testgrid-group-dashboard .groupName=${params.group}></testgrid-group-dashboard>`,
    },
    {
      path: '/dashboards/:dashboard/*', 
      render: (params: RouteParameter) => html`<testgrid-dashboard-grid .dashboardName=${params.dashboard} .tabName=${params[0]} showTab></testgrid-dashboard-grid>`,
    },
    {
      path: '/dashboards/:dashboard', 
      render: (params: RouteParameter) => html`<testgrid-dashboard-grid .dashboardName=${params.dashboard}></testgrid-dashboard-grid>`,
    },
    {
      path: '/',
      render: () => html`<testgrid-index></testgrid-index>`,
    },
  ])

  /**
   * Lit-element lifecycle method.
   * Invoked when a component is added to the document's DOM.
   */
  connectedCallback(){
    super.connectedCallback();
    window.addEventListener('location-changed', () => {
      this.router.goto(location.pathname);
    });
  }

  /**
   * Lit-element lifecycle method.
   * Invoked on each update to perform rendering tasks.
   */
  render(){
    return html`${this.router.outlet()}`;
  }
}

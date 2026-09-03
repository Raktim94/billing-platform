import { createRootRoute, createRoute, createRouter, Navigate, Outlet, redirect, useSearch } from "@tanstack/react-router";
import { AppShell } from "./components/AppShell";
import { DashboardPage } from "./pages/DashboardPage";
import { LoginPage } from "./pages/LoginPage";
import { BootstrapPage } from "./pages/BootstrapPage";
import { PlaceholderPage } from "./pages/PlaceholderPage";
import { BillingPage } from "./pages/sales/BillingPage";
import { SalesDetailPage } from "./pages/sales/SalesDetailPage";
import { SalesListPage } from "./pages/sales/SalesListPage";
import { readSessionHint } from "./auth/session";

/**
 * Code-based routing (not TanStack Router's file-based/codegen mode) —
 * simplest option for this pass's route count, no extra Vite plugin or
 * generated route-tree file needed.
 *
 * Every authenticated route is a DIRECT child of the root, each wrapped
 * in <AppShell> individually via `withShell` below, rather than nested
 * under one shared layout route with a shared `beforeLoad`. Root cause of
 * why a shared-layout attempt kept failing (confirmed via a literal
 * `<Link to="/sales">` test, isolated from the nav-list array): the
 * `placeholderRoute(path: string, title: string)` factory's plain
 * `string` parameter widened each route's literal path type before
 * `createRoute` ever saw it — TanStack Router's type system needs the
 * literal ("/sales"), not `string`, to add a path to the router's
 * registered union. Fixed properly below with
 * `placeholderRoute<TPath extends string>(path: TPath, ...)`, which
 * preserves the literal through the call. Kept this flat structure
 * (one `beforeLoad: requireAuth` per route) rather than reintroducing a
 * shared layout parent now that the real fix is known, since it already
 * works and re-nesting isn't worth the risk under this pass's time
 * budget — worth revisiting in a later pass if the per-route repetition
 * becomes annoying.
 */

const rootRoute = createRootRoute({
  component: () => <Outlet />,
});

// Session hint is advisory only (see auth/session.ts) — a stale hint
// just means the first protected API call 401s and the app-level
// listener bounces back to /login. This guard exists purely to avoid
// flashing the authenticated shell for a definitely-logged-out visitor.
function requireAuth() {
  if (!readSessionHint()) {
    throw redirect({ to: "/login" });
  }
}

function withShell(Page: () => React.ReactElement) {
  return () => (
    <AppShell>
      <Page />
    </AppShell>
  );
}

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  component: LoginPage,
});

const bootstrapRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/setup",
  component: BootstrapPage,
});

const dashboardRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  beforeLoad: requireAuth,
  component: withShell(DashboardPage),
});

function placeholderRoute<TPath extends string>(path: TPath, title: string) {
  return createRoute({
    getParentRoute: () => rootRoute,
    path,
    beforeLoad: requireAuth,
    component: withShell(() => <PlaceholderPage title={title} />),
  });
}

const salesListRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sales",
  beforeLoad: requireAuth,
  component: withShell(SalesListPage),
});

const salesNewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sales/new",
  beforeLoad: requireAuth,
  validateSearch: (search: Record<string, unknown>): { resume?: string } => ({
    resume: typeof search.resume === "string" ? search.resume : undefined,
  }),
  component: withShell(() => {
    const { resume } = useSearch({ from: salesNewRoute.id });
    return <BillingPage resumeDocumentId={resume} />;
  }),
});

const salesDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sales/$id",
  beforeLoad: requireAuth,
  component: withShell(() => {
    const { id } = salesDetailRoute.useParams();
    return <SalesDetailPage id={id} />;
  }),
});

const purchasesRoute = placeholderRoute("/purchases", "Purchases");
const inventoryRoute = placeholderRoute("/inventory", "Inventory");
const contactsRoute = placeholderRoute("/contacts", "Contacts");
const accountingRoute = placeholderRoute("/accounting", "Accounting");
const gstRoute = placeholderRoute("/gst", "GST / Tax");
const reportsRoute = placeholderRoute("/reports", "Reports");
const integrationsRoute = placeholderRoute("/integrations", "Integrations");
const settingsRoute = placeholderRoute("/settings", "Settings");

const notFoundRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "$",
  component: () => <Navigate to="/" />,
});

const routeTree = rootRoute.addChildren([
  loginRoute,
  bootstrapRoute,
  dashboardRoute,
  salesListRoute,
  salesNewRoute,
  salesDetailRoute,
  purchasesRoute,
  inventoryRoute,
  contactsRoute,
  accountingRoute,
  gstRoute,
  reportsRoute,
  integrationsRoute,
  settingsRoute,
  notFoundRoute,
]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

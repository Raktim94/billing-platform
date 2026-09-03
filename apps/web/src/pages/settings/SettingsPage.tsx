import { Link } from "@tanstack/react-router";
import { useOrgContext } from "../../lib/useOrgContext";
import layout from "../DashboardPage.module.css";

export function SettingsPage() {
  const org = useOrgContext();

  return (
    <div className={layout.page}>
      <div className={layout.heading}>
        <div>
          <h1>Settings</h1>
          <p className={layout.subtitle}>Your business, legal entity, branch, and warehouse.</p>
        </div>
      </div>

      <div className={layout.panel}>
        {org.isPending ? (
          <div className={layout.skeleton} style={{ height: 160 }} aria-hidden="true" />
        ) : org.isError ? (
          <p className={layout.errorState} role="alert">
            Couldn't load your business details.
          </p>
        ) : (
          <dl style={{ display: "grid", gridTemplateColumns: "180px 1fr", rowGap: 12 }}>
            <dt className={layout.subtitle}>Business</dt>
            <dd>{org.organisation?.Name}</dd>
            <dt className={layout.subtitle}>Legal entity</dt>
            <dd>{org.legalEntity?.LegalName}</dd>
            <dt className={layout.subtitle}>GSTIN</dt>
            <dd>{org.legalEntity?.GSTIN || "—"}</dd>
            <dt className={layout.subtitle}>Branch</dt>
            <dd>{org.branch?.Name}</dd>
            <dt className={layout.subtitle}>Warehouse</dt>
            <dd>{org.warehouse?.Name}</dd>
            <dt className={layout.subtitle}>Currency</dt>
            <dd>{org.organisation?.DefaultCurrencyCode}</dd>
          </dl>
        )}
      </div>

      <div className={layout.panel}>
        <h2>GST &amp; e-Way Bill</h2>
        <p className={layout.emptyState} style={{ textAlign: "left", padding: 0 }}>
          Tax rates, e-Way Bill mode, vehicles, and transporters live on the{" "}
          <Link to="/gst">GST / Tax</Link> page.
        </p>
      </div>
    </div>
  );
}

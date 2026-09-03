import { ReportTable } from "../../components/ReportTable";
import layout from "../DashboardPage.module.css";

export function ReportsPage() {
  return (
    <div className={layout.page}>
      <div className={layout.heading}>
        <div>
          <h1>Reports</h1>
          <p className={layout.subtitle}>Sales, purchases, and stock movement — the last 30 days by default.</p>
        </div>
      </div>

      <div className={layout.panel}>
        <h2>Sales invoices</h2>
        <ReportTable path="/reports/sales/invoices?format=json" />
      </div>
      <div className={layout.panel}>
        <h2>Gross profit</h2>
        <ReportTable path="/reports/sales/gross-profit?format=json" />
      </div>
      <div className={layout.panel}>
        <h2>Purchase summary</h2>
        <ReportTable path="/reports/purchases/summary?format=json" />
      </div>
      <div className={layout.panel}>
        <h2>Stock movements</h2>
        <ReportTable path="/reports/inventory/movements?format=json" emptyLabel="No stock movements recorded yet." />
      </div>
    </div>
  );
}

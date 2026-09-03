import { ReportTable } from "../../components/ReportTable";
import layout from "../DashboardPage.module.css";

export function InventoryPage() {
  return (
    <div className={layout.page}>
      <div className={layout.heading}>
        <div>
          <h1>Inventory</h1>
          <p className={layout.subtitle}>Current stock value and items running low.</p>
        </div>
      </div>

      <div className={layout.panel}>
        <h2>Low stock</h2>
        <ReportTable path="/reports/inventory/low-stock?format=json" emptyLabel="Nothing is running low." />
      </div>

      <div className={layout.panel}>
        <h2>Stock valuation</h2>
        <ReportTable path="/reports/inventory/valuation?format=json" emptyLabel="No stock recorded yet." />
      </div>
    </div>
  );
}

import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api-client";
import layout from "../pages/DashboardPage.module.css";
import ui from "./ui.module.css";

/** internal/platform/export.Table — every /reports/* endpoint's shape
 * (format=json, the default) — title/headers/rows, rows pre-stringified.
 * One generic renderer for every report table in the app, so each screen
 * (Inventory, Purchases, Accounting, GST) just points this at a path
 * instead of hand-building a table per report. */
interface ReportTableData {
  title: string;
  headers: string[];
  rows: string[][];
}

export function ReportTable({ path, emptyLabel }: { path: string; emptyLabel?: string }) {
  const query = useQuery({
    queryKey: ["report-table", path],
    queryFn: () => api.get<ReportTableData>(path),
  });

  if (query.isPending) {
    return <div className={layout.skeleton} style={{ height: 120 }} aria-hidden="true" />;
  }
  if (query.isError) {
    return (
      <p className={layout.errorState} role="alert">
        Couldn't load this report.
      </p>
    );
  }
  if (query.data.rows.length === 0) {
    return <p className={layout.emptyState}>{emptyLabel ?? "Nothing to show yet."}</p>;
  }
  return (
    <div className={ui.tableScroll}>
      <table className={ui.table}>
        <thead>
          <tr>
            {query.data.headers.map((h, i) => (
              <th key={i}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {query.data.rows.map((row, i) => (
            <tr key={i}>
              {row.map((cell, j) => (
                <td key={j} className={j > 0 ? "num" : undefined}>
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

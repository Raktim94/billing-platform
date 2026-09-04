import { useQuery } from "@tanstack/react-query";
import ReactECharts from "echarts-for-react";
import styles from "./DashboardPage.module.css";
import { QuickAccess } from "../components/QuickAccess";
import { StatCard } from "../components/StatCard";
import { api } from "../lib/api-client";
import { formatMoney, isZeroMoney, moneyToApproxNumber, type Money } from "../lib/money";
import { useTheme } from "../theme/ThemeProvider";

/** Mirrors internal/modules/reporting/domain.DashboardSummary exactly —
 * that struct has no json tags, so field names serialize verbatim as Go's
 * exported names (checked against internal/modules/reporting/domain/domain.go). */
interface DashboardSummary {
  TodaySales: Money;
  TodayCollections: Money;
  TodayPurchases: Money;
  OutstandingReceivable: Money;
  OutstandingPayable: Money;
  CurrentStockValue: Money;
  LowStockCount: number;
  OverdueReceivable: Money;
}

/** internal/modules/reporting/httpapi's export.Table shape for
 * GET /reports/sales/summary?format=json&group_by=day — rows are
 * pre-stringified [Key, DocumentCount, Taxable, Tax, GrandTotal]. */
interface ReportTable {
  title: string;
  headers: string[];
  rows: string[][];
}

function useDashboard() {
  return useQuery({
    queryKey: ["dashboard"],
    queryFn: () => api.get<DashboardSummary>("/reports/dashboard"),
  });
}

function useSalesTrend() {
  return useQuery({
    queryKey: ["reports", "sales-summary", "day"],
    queryFn: () => api.get<ReportTable>("/reports/sales/summary?group_by=day&format=json"),
  });
}

export function DashboardPage() {
  const dashboard = useDashboard();
  const trend = useSalesTrend();
  const { theme } = useTheme();

  return (
    <div className={styles.page}>
      <div className={styles.heading}>
        <div>
          <h1>Dashboard</h1>
          <p className={styles.subtitle}>Today's business, at a glance.</p>
        </div>
      </div>

      <QuickAccess />

      {dashboard.isError ? (
        <div className={styles.errorState} role="alert">
          Couldn't load today's summary. Check your connection and try again.
        </div>
      ) : dashboard.isPending ? (
        <div className={styles.cardGrid}>
          {Array.from({ length: 8 }, (_, i) => (
            <div key={i} className={styles.skeleton} aria-hidden="true" />
          ))}
        </div>
      ) : (
        <div className={styles.cardGrid}>
          <StatCard label="Today's sales" value={formatMoney(dashboard.data.TodaySales)} />
          <StatCard
            label="Today's collections"
            value={formatMoney(dashboard.data.TodayCollections)}
            polarity={isZeroMoney(dashboard.data.TodayCollections) ? "neutral" : "positive"}
          />
          <StatCard label="Today's purchases" value={formatMoney(dashboard.data.TodayPurchases)} />
          <StatCard
            label="Outstanding receivable"
            value={formatMoney(dashboard.data.OutstandingReceivable)}
            polarity={isZeroMoney(dashboard.data.OutstandingReceivable) ? "neutral" : "warning"}
          />
          <StatCard
            label="Outstanding payable"
            value={formatMoney(dashboard.data.OutstandingPayable)}
            polarity={isZeroMoney(dashboard.data.OutstandingPayable) ? "neutral" : "warning"}
          />
          <StatCard label="Current stock value" value={formatMoney(dashboard.data.CurrentStockValue)} />
          <StatCard
            label="Low stock items"
            value={String(dashboard.data.LowStockCount)}
            polarity={dashboard.data.LowStockCount > 0 ? "warning" : "neutral"}
          />
          <StatCard
            label="Overdue receivable"
            value={formatMoney(dashboard.data.OverdueReceivable)}
            polarity={isZeroMoney(dashboard.data.OverdueReceivable) ? "neutral" : "negative"}
          />
        </div>
      )}

      <div className={styles.panel}>
        <h2>Sales trend</h2>
        {trend.isError ? (
          <div className={styles.errorState} role="alert">
            Couldn't load the sales trend.
          </div>
        ) : trend.isPending ? (
          <div className={styles.skeleton} style={{ height: 260 }} aria-hidden="true" />
        ) : (trend.data.rows ?? []).length === 0 ? (
          <p className={styles.emptyState}>
            No sales recorded yet. Once you create and finalize an invoice, its trend will show up here.
          </p>
        ) : (
          <SalesTrendChart rows={trend.data.rows ?? []} dark={theme === "dark"} />
        )}
      </div>
    </div>
  );
}

function SalesTrendChart({ rows, dark }: { rows: string[][]; dark: boolean }) {
  const accent = dark ? "#29c191" : "#0f6e5c";
  const textColor = dark ? "#9db0a4" : "#5b6b62";
  const gridColor = dark ? "#2b3632" : "#dbdfd8";

  const days = rows.map((r) => r[0] ?? "");
  const totals = rows.map((r) => moneyToApproxNumber({ amount: r[4] ?? "0", currency: "INR" }));

  // echarts-for-react renders to a bare <canvas> with no text alternative
  // — a screen-reader user gets nothing from it (WCAG 1.1.1). This table
  // carries the same day/total data as real, readable markup; the chart
  // itself is hidden from assistive tech below so the two aren't both
  // announced.
  const accessibleTable = (
    <table className="srOnly">
      <caption>Daily sales total, most recent {rows.length} day(s)</caption>
      <thead>
        <tr>
          <th scope="col">Date</th>
          <th scope="col">Total</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={r[0] ?? i}>
            <td>{r[0]}</td>
            <td>{formatMoney({ amount: r[4] ?? "0", currency: "INR" })}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );

  const option = {
    grid: { left: 48, right: 16, top: 24, bottom: 32 },
    xAxis: {
      type: "category" as const,
      data: days,
      axisLine: { lineStyle: { color: gridColor } },
      axisLabel: { color: textColor, fontFamily: "IBM Plex Sans" },
    },
    yAxis: {
      type: "value" as const,
      splitLine: { lineStyle: { color: gridColor } },
      axisLabel: { color: textColor, fontFamily: "IBM Plex Mono" },
    },
    tooltip: { trigger: "axis" as const },
    series: [
      {
        type: "line" as const,
        data: totals,
        color: accent,
        smooth: false,
        symbolSize: 6,
        lineStyle: { width: 2 },
        areaStyle: { opacity: 0.08, color: accent },
      },
    ],
  };

  return (
    <>
      {accessibleTable}
      <div aria-hidden="true">
        <ReactECharts option={option} style={{ height: 260 }} notMerge />
      </div>
    </>
  );
}

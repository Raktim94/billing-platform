import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import ui from "../../components/ui.module.css";
import { api } from "../../lib/api-client";
import { formatMoney } from "../../lib/money";
import layout from "../DashboardPage.module.css";
import { DOCUMENT_TYPE_LABELS, type SalesDocument } from "./types";

function statusTone(status: SalesDocument["Status"]) {
  if (status === "FINALIZED") return "positive";
  if (status === "CANCELLED") return "negative";
  return "warning";
}

export function SalesListPage() {
  const documents = useQuery({
    queryKey: ["sales-documents"],
    queryFn: () => api.getListField<SalesDocument>("/sales/documents", "documents"),
  });

  return (
    <div className={layout.page}>
      <div className={layout.heading}>
        <div>
          <h1>Sales</h1>
          <p className={layout.subtitle}>Every quotation, order, and invoice — draft or finalized.</p>
        </div>
        <Link to="/sales/new" className={ui.btnPrimary}>
          + New sale
        </Link>
      </div>

      <div className={layout.panel}>
        {documents.isError ? (
          <p className={layout.errorState} role="alert">
            Couldn't load sales documents.
          </p>
        ) : documents.isPending ? (
          <div className={layout.skeleton} style={{ height: 240 }} aria-hidden="true" />
        ) : documents.data.length === 0 ? (
          <p className={layout.emptyState}>No sales yet — start your first sale above.</p>
        ) : (
          <div className={ui.tableScroll}>
            <table className={ui.table}>
              <thead>
                <tr>
                  <th scope="col">Number</th>
                  <th scope="col">Type</th>
                  <th scope="col">Status</th>
                  <th scope="col">Date</th>
                  <th scope="col">Total</th>
                </tr>
              </thead>
              <tbody>
                {documents.data.map((d) => (
                  <tr key={d.ID}>
                    <td>
                      <Link
                        to={d.Status === "DRAFT" ? "/sales/new" : "/sales/$id"}
                        params={d.Status === "DRAFT" ? undefined : { id: d.ID }}
                        search={d.Status === "DRAFT" ? { resume: d.ID } : undefined}
                        className={ui.linkRow}
                      >
                        {d.DocumentNumber || "(draft)"}
                      </Link>
                    </td>
                    <td>{DOCUMENT_TYPE_LABELS[d.DocumentType]}</td>
                    <td>
                      <span className={ui.badge} data-tone={statusTone(d.Status)}>
                        {d.Status}
                      </span>
                    </td>
                    <td>{new Date(d.IssueDate).toLocaleDateString()}</td>
                    <td className="num">{d.GrandTotalAmount ? formatMoney(d.GrandTotalAmount) : "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

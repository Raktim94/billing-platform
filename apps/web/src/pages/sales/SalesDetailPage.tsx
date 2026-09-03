import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { EwayBillCard } from "../../components/EwayBillCard";
import ui from "../../components/ui.module.css";
import { api } from "../../lib/api-client";
import { formatMoney } from "../../lib/money";
import layout from "../DashboardPage.module.css";
import styles from "./SalesDetailPage.module.css";
import { DOCUMENT_TYPE_LABELS, type SalesDocument, type SalesDocumentLine } from "./types";

const EWB_ELIGIBLE_TYPES = new Set(["TAX_INVOICE", "POS_INVOICE", "DELIVERY_CHALLAN", "SALES_RETURN"]);

export function SalesDetailPage({ id }: { id: string }) {
  const doc = useQuery({
    queryKey: ["sales-document", id],
    queryFn: () => api.get<{ document: SalesDocument; lines: SalesDocumentLine[] }>(`/sales/documents/${id}`),
  });

  if (doc.isPending) {
    return (
      <div className={layout.page}>
        <div className={layout.skeleton} style={{ height: 320 }} aria-hidden="true" />
      </div>
    );
  }

  if (doc.isError) {
    return (
      <div className={layout.page}>
        <p className={layout.errorState} role="alert">
          Couldn't load this sale.
        </p>
      </div>
    );
  }

  const { document, lines } = doc.data;

  return (
    <div className={layout.page}>
      <div className={layout.heading}>
        <div>
          <h1>{document.DocumentNumber || "Draft sale"}</h1>
          <p className={layout.subtitle}>{DOCUMENT_TYPE_LABELS[document.DocumentType]}</p>
        </div>
        {document.Status === "DRAFT" ? (
          <Link to="/sales/new" search={{ resume: document.ID }} className={ui.btnPrimary}>
            Continue billing
          </Link>
        ) : (
          <a href={`/api/v1/sales/documents/${document.ID}/print`} target="_blank" rel="noopener noreferrer" className={ui.btnSecondary}>
            Print / Download PDF
          </a>
        )}
      </div>

      <div className={styles.grid}>
        <div className={layout.panel}>
          <div className={styles.metaRow}>
            <span>
              Status: <strong>{document.Status}</strong>
            </span>
            <span>
              Issue date: <strong>{new Date(document.IssueDate).toLocaleDateString()}</strong>
            </span>
            <span>
              Place of supply: <strong>{document.PlaceOfSupplyStateCode}</strong>
            </span>
          </div>

          {lines.length === 0 ? (
            <p className={layout.emptyState}>No items on this document.</p>
          ) : (
            <div className={ui.tableScroll}>
              <table className={ui.table}>
                <thead>
                  <tr>
                    <th scope="col">#</th>
                    <th scope="col">HSN/SAC</th>
                    <th scope="col">Qty</th>
                    <th scope="col">Rate</th>
                    <th scope="col">Total</th>
                  </tr>
                </thead>
                <tbody>
                  {lines.map((l) => (
                    <tr key={l.ID}>
                      <td className="num">{l.LineNumber}</td>
                      <td>{l.HSNSACCode}</td>
                      <td className="num">{l.Quantity}</td>
                      <td className="num">{formatMoney(l.UnitPrice)}</td>
                      <td className="num">{formatMoney(l.LineTotal)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <div className={styles.totalRow}>
            <span>Grand total</span>
            <span className="num">{document.GrandTotalAmount ? formatMoney(document.GrandTotalAmount) : "—"}</span>
          </div>
        </div>

        {document.Status === "FINALIZED" && EWB_ELIGIBLE_TYPES.has(document.DocumentType) ? (
          <EwayBillCard documentId={document.ID} />
        ) : null}
      </div>
    </div>
  );
}

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import ui from "../../components/ui.module.css";
import { api, ApiError } from "../../lib/api-client";
import { formatMoney } from "../../lib/money";
import type { Party } from "../../lib/partyTypes";
import { useOrgContext } from "../../lib/useOrgContext";
import layout from "../DashboardPage.module.css";
import styles from "./BillingPage.module.css";
import { DOCUMENT_TYPE_LABELS, type DocumentType, type SalesDocument, type SalesDocumentLine } from "./types";

interface BillingLookupResult {
  ProductID: string;
  ProductName: string;
  HSNSACCode: string;
  ProductVariantID: string;
  SKUCode: string;
  QuantityOnHand: string;
  QuantityAvailable: string;
  UnitPrice: { amount: string; currency: string } | null;
}

interface AgeingBucket {
  Total: { amount: string; currency: string };
}

/** The billing counter — brief's "exceptional attention" screen. A sale is
 * a real DRAFT sales_documents row from the moment the customer is picked
 * (not client-side-only state until some later "save"): every add-line
 * call is a real, immediately-persisted API call. That is what makes
 * "hold" free — navigating away just leaves a DRAFT sitting in the Sales
 * list, and "restore" is just opening that same document again
 * (SalesDetailPage's "Continue billing" button routes back here with the
 * existing id).
 */
export function BillingPage({ resumeDocumentId }: { resumeDocumentId?: string }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const org = useOrgContext();

  const [documentId, setDocumentId] = useState<string | undefined>(resumeDocumentId);
  const [documentType, setDocumentType] = useState<DocumentType>("TAX_INVOICE");
  const [customerQuery, setCustomerQuery] = useState("");
  const [customer, setCustomer] = useState<Party | null>(null);
  const [showCustomerResults, setShowCustomerResults] = useState(false);

  const [productQuery, setProductQuery] = useState("");
  const [productResults, setProductResults] = useState<BillingLookupResult[]>([]);
  const searchInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!showCustomerResults) return;
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setShowCustomerResults(false);
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [showCustomerResults]);

  const doc = useQuery({
    queryKey: ["sales-document", documentId],
    queryFn: () => api.get<{ document: SalesDocument; lines: SalesDocumentLine[] }>(`/sales/documents/${documentId}`),
    enabled: !!documentId,
  });

  // Resuming a held draft: hydrate the customer field from the loaded
  // document once, so the header reads correctly without a second lookup
  // UI — the customer picker above is naturally disabled once a document
  // exists (see below), so this is display-only.
  useEffect(() => {
    if (resumeDocumentId && doc.data && !customer) {
      setCustomer({ ID: doc.data.document.CustomerPartyID } as Party);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [doc.data]);

  const customerSearch = useQuery({
    queryKey: ["party-search", customerQuery],
    queryFn: () => api.get<{ parties: Party[] }>(`/contacts/parties?q=${encodeURIComponent(customerQuery)}`),
    enabled: customerQuery.length >= 2 && !documentId,
  });

  const customerAgeing = useQuery({
    queryKey: ["party-ageing", customer?.ID],
    queryFn: () => api.get<AgeingBucket>(`/accounting/parties/${customer?.ID}/ageing`),
    enabled: !!customer && customer.ID !== resumeDocumentId,
  });

  useEffect(() => {
    if (productQuery.trim().length < 2 || !documentId) {
      setProductResults([]);
      return;
    }
    const handle = setTimeout(() => {
      const params = new URLSearchParams({ q: productQuery });
      if (org.warehouse) params.set("warehouse_id", org.warehouse.ID);
      api
        .get<{ results: BillingLookupResult[] }>(`/sales/billing-lookup?${params.toString()}`)
        .then((res) => setProductResults(res.results))
        .catch(() => setProductResults([]));
    }, 150);
    return () => clearTimeout(handle);
  }, [productQuery, documentId, org.warehouse]);

  const startSale = useMutation({
    mutationFn: async () => {
      if (!customer || !org.legalEntity || !org.branch || !org.warehouse || !org.organisation) {
        throw new Error("Missing organisation context.");
      }
      return api.post<SalesDocument>("/sales/documents", {
        legal_entity_id: org.legalEntity.ID,
        branch_id: org.branch.ID,
        warehouse_id: org.warehouse.ID,
        customer_party_id: customer.ID,
        document_type: documentType,
        place_of_supply_state_code: org.legalEntity.GSTStateCode || "00",
        currency_code: org.organisation.DefaultCurrencyCode || "INR",
        base_currency_code: org.organisation.DefaultCurrencyCode || "INR",
        exchange_rate: "1",
        pricing_mode: "EXCLUSIVE",
      });
    },
    onSuccess: (d) => {
      setDocumentId(d.ID);
      setTimeout(() => searchInputRef.current?.focus(), 0);
    },
  });

  const addLine = useMutation({
    mutationFn: async (vars: { productVariantId: string; unitId: string; quantity: string; unitPrice: string }) =>
      api.post(`/sales/documents/${documentId}/lines`, {
        product_variant_id: vars.productVariantId,
        unit_id: vars.unitId,
        quantity: vars.quantity,
        unit_price: vars.unitPrice,
        line_discount_amount: "0",
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sales-document", documentId] });
      setProductQuery("");
      setProductResults([]);
      searchInputRef.current?.focus();
    },
  });

  const finalize = useMutation({
    mutationFn: () => api.post<SalesDocument>(`/sales/documents/${documentId}/finalize`),
    onSuccess: (d) => navigate({ to: "/sales/$id", params: { id: d.ID } }),
  });

  async function handleAddProduct(result: BillingLookupResult) {
    let unitId: string;
    try {
      const product = await api.get<{ ID: string; BaseUOMID: string }>(`/catalogue/products/${result.ProductID}`);
      unitId = product.BaseUOMID;
    } catch {
      return;
    }
    addLine.mutate({
      productVariantId: result.ProductVariantID,
      unitId,
      quantity: "1",
      unitPrice: result.UnitPrice?.amount ?? "0",
    });
  }

  const lines = doc.data?.lines ?? [];
  const grandTotal = doc.data?.document.GrandTotalAmount;

  return (
    <div className={layout.page}>
      <div className={layout.heading}>
        <div>
          <h1>{resumeDocumentId ? "Continue sale" : "New sale"}</h1>
          <p className={layout.subtitle}>Scan a barcode or search by product name — stock and price show instantly.</p>
        </div>
      </div>

      <div className={layout.panel}>
        <div className={styles.headerGrid}>
          <div className={ui.field}>
            <label htmlFor="doc-type">Document type</label>
            <select
              id="doc-type"
              className={ui.select}
              value={documentType}
              disabled={!!documentId}
              onChange={(e) => setDocumentType(e.target.value as DocumentType)}
            >
              {(["TAX_INVOICE", "POS_INVOICE", "QUOTATION", "SALES_ORDER"] as DocumentType[]).map((t) => (
                <option key={t} value={t}>
                  {DOCUMENT_TYPE_LABELS[t]}
                </option>
              ))}
            </select>
          </div>

          <div className={`${ui.field} ${styles.customerField}`}>
            <label htmlFor="customer-search">Customer</label>
            {customer && documentId ? (
              <div className={styles.customerBadge}>
                <strong>{customer.LegalName || customer.ID}</strong>
              </div>
            ) : customer ? (
              <div className={styles.customerBadge}>
                <strong>{customer.LegalName}</strong>
                {customerAgeing.data ? (
                  <span className={styles.customerBalance}>
                    Outstanding: {formatMoney(customerAgeing.data.Total)}
                    {customer.CreditLimitAmount ? ` · Credit limit: ₹${customer.CreditLimitAmount}` : ""}
                  </span>
                ) : null}
                <button type="button" className={ui.btnSecondary} onClick={() => setCustomer(null)}>
                  Change
                </button>
              </div>
            ) : (
              <div className={styles.customerSearchWrap}>
                <input
                  id="customer-search"
                  className={ui.input}
                  placeholder="Search customer by name or phone…"
                  value={customerQuery}
                  onChange={(e) => {
                    setCustomerQuery(e.target.value);
                    setShowCustomerResults(true);
                  }}
                  onFocus={() => setShowCustomerResults(true)}
                  autoComplete="off"
                />
                {showCustomerResults && customerSearch.data?.parties.length ? (
                  <ul className={styles.dropdown} role="menu" aria-label="Customer results">
                    {customerSearch.data.parties.map((p) => (
                      <li key={p.ID}>
                        <button
                          type="button"
                          role="menuitem"
                          className={styles.dropdownItem}
                          onClick={() => {
                            setCustomer(p);
                            setShowCustomerResults(false);
                          }}
                        >
                          {p.LegalName} {p.Phone ? <span className={ui.muted}>· {p.Phone}</span> : null}
                        </button>
                      </li>
                    ))}
                  </ul>
                ) : null}
              </div>
            )}
          </div>

          {!documentId ? (
            <button
              type="button"
              className={ui.btnPrimary}
              disabled={!customer || org.isPending || startSale.isPending}
              onClick={() => startSale.mutate()}
            >
              {startSale.isPending ? "Starting…" : "Start sale"}
            </button>
          ) : null}
        </div>
        {startSale.isError ? (
          <p className={styles.errorText} role="alert">
            {startSale.error instanceof ApiError ? startSale.error.message : "Could not start this sale."}
          </p>
        ) : null}
        {org.isError ? (
          <p className={styles.errorText} role="alert">
            Could not load your branch/warehouse setup. Check Settings.
          </p>
        ) : null}
      </div>

      {documentId ? (
        <>
          <div className={layout.panel}>
            <label htmlFor="product-search" className={styles.searchLabel}>
              Search or scan a product
            </label>
            <input
              id="product-search"
              ref={searchInputRef}
              className={ui.input}
              placeholder="Type a product name, or scan a barcode…"
              value={productQuery}
              onChange={(e) => setProductQuery(e.target.value)}
              autoComplete="off"
            />
            {productResults.length > 0 ? (
              <ul className={styles.productList}>
                {productResults.map((r) => (
                  <li key={r.ProductVariantID} className={styles.productRow}>
                    <div>
                      <strong>{r.ProductName}</strong>
                      <div className={ui.muted}>
                        SKU {r.SKUCode} · In stock: {r.QuantityAvailable || "0"}
                      </div>
                    </div>
                    <div className={styles.productPrice}>{r.UnitPrice ? formatMoney(r.UnitPrice) : "—"}</div>
                    <button
                      type="button"
                      className={ui.btnPrimary}
                      disabled={addLine.isPending}
                      onClick={() => handleAddProduct(r)}
                    >
                      Add
                    </button>
                  </li>
                ))}
              </ul>
            ) : null}
          </div>

          <div className={layout.panel}>
            <h2>Items ({lines.length})</h2>
            {lines.length === 0 ? (
              <p className={layout.emptyState}>No items yet — search above to add the first one.</p>
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
              <span className="num">{grandTotal ? formatMoney(grandTotal) : "—"}</span>
            </div>
            <div className={ui.formActions}>
              <button type="button" className={ui.btnSecondary} onClick={() => navigate({ to: "/sales" })}>
                Hold for later
              </button>
              <button
                type="button"
                className={ui.btnPrimary}
                disabled={lines.length === 0 || finalize.isPending}
                onClick={() => finalize.mutate()}
              >
                {finalize.isPending ? "Finalizing…" : "Finalize sale"}
              </button>
            </div>
            {finalize.isError ? (
              <p className={styles.errorText} role="alert">
                {finalize.error instanceof ApiError ? finalize.error.message : "Could not finalize this sale."}
              </p>
            ) : null}
          </div>
        </>
      ) : null}
    </div>
  );
}

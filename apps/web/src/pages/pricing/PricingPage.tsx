import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import ui from "../../components/ui.module.css";
import { api, ApiError } from "../../lib/api-client";
import { formatMoney } from "../../lib/money";
import { useOrgContext } from "../../lib/useOrgContext";
import layout from "../DashboardPage.module.css";

interface PriceList {
  ID: string;
  Name: string;
  CurrencyCode: string;
  IsDefault: boolean;
  Status: string;
}
interface PriceListItem {
  ID: string;
  ProductVariantID: string;
  UnitID: string;
  Price: { amount: string; currency: string };
}
interface Product {
  ID: string;
  Name: string;
  BaseUOMID: string;
}
interface ProductVariant {
  ID: string;
  SKUCode: string;
}

export function PricingPage() {
  const queryClient = useQueryClient();
  const org = useOrgContext();
  const [selectedListId, setSelectedListId] = useState<string | null>(null);
  const [showListForm, setShowListForm] = useState(false);
  const [listName, setListName] = useState("");
  const [listIsDefault, setListIsDefault] = useState(false);

  const priceLists = useQuery({
    queryKey: ["price-lists"],
    queryFn: () => api.getListField<PriceList>("/pricing/price-lists", "price_lists"),
  });

  const createPriceList = useMutation({
    mutationFn: () =>
      api.post<PriceList>("/pricing/price-lists", {
        name: listName,
        currency_code: org.organisation?.DefaultCurrencyCode || "INR",
        is_default: listIsDefault,
      }),
    onSuccess: (pl) => {
      queryClient.invalidateQueries({ queryKey: ["price-lists"] });
      setListName("");
      setListIsDefault(false);
      setShowListForm(false);
      setSelectedListId(pl.ID);
    },
  });

  const selected = priceLists.data?.find((pl) => pl.ID === selectedListId) ?? null;

  return (
    <div className={layout.page}>
      <div className={layout.heading}>
        <div>
          <h1>Pricing</h1>
          <p className={layout.subtitle}>Set what each product sells for — the billing screen uses these prices automatically.</p>
        </div>
        <button type="button" className={ui.btnPrimary} onClick={() => setShowListForm((v) => !v)}>
          + New price list
        </button>
      </div>

      {showListForm ? (
        <div className={layout.panel}>
          <div className={ui.formGrid}>
            <div className={ui.field}>
              <label htmlFor="price-list-name">Name</label>
              <input
                id="price-list-name"
                className={ui.input}
                value={listName}
                onChange={(e) => setListName(e.target.value)}
                placeholder="e.g. Retail"
              />
            </div>
            <label style={{ display: "flex", gap: 8, alignItems: "center" }}>
              <input type="checkbox" checked={listIsDefault} onChange={(e) => setListIsDefault(e.target.checked)} />
              Use as the default price list for billing
            </label>
            <button
              type="button"
              className={ui.btnPrimary}
              disabled={!listName || createPriceList.isPending}
              onClick={() => createPriceList.mutate()}
            >
              Save price list
            </button>
          </div>
          {createPriceList.isError ? (
            <p role="alert" style={{ color: "var(--color-negative)", marginTop: 8 }}>
              {createPriceList.error instanceof ApiError ? createPriceList.error.message : "Could not save this price list."}
            </p>
          ) : null}
        </div>
      ) : null}

      <div className={layout.panel}>
        <h2>Price lists</h2>
        {priceLists.isError ? (
          <p className={layout.errorState} role="alert">
            Couldn't load price lists.
          </p>
        ) : priceLists.isPending ? (
          <div className={layout.skeleton} style={{ height: 80 }} aria-hidden="true" />
        ) : priceLists.data.length === 0 ? (
          <p className={layout.emptyState}>No price lists yet — add one above to start pricing your products.</p>
        ) : (
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            {priceLists.data.map((pl) => (
              <button
                key={pl.ID}
                type="button"
                className={pl.ID === selectedListId ? ui.btnPrimary : ui.btnSecondary}
                onClick={() => setSelectedListId(pl.ID)}
              >
                {pl.Name} {pl.IsDefault ? "(default)" : ""}
              </button>
            ))}
          </div>
        )}
      </div>

      {selected ? <PriceListItemsPanel priceList={selected} /> : null}
    </div>
  );
}

function PriceListItemsPanel({ priceList }: { priceList: PriceList }) {
  const queryClient = useQueryClient();
  const [productQuery, setProductQuery] = useState("");
  const [productResults, setProductResults] = useState<Product[]>([]);
  const [selectedProduct, setSelectedProduct] = useState<Product | null>(null);
  const [amount, setAmount] = useState("");

  const items = useQuery({
    queryKey: ["price-list-items", priceList.ID],
    queryFn: () => api.getListField<PriceListItem>(`/pricing/price-lists/${priceList.ID}/items`, "items"),
  });

  // Items only carry ProductVariantID/UnitID — resolve a display name/SKU
  // by mapping every product's variants once. Fine at this app's target
  // scale (a single small-business catalogue, not an enterprise SKU
  // count); revisit if that assumption stops holding.
  const allProducts = useQuery({
    queryKey: ["products-for-pricing"],
    queryFn: () => api.getListField<Product>("/catalogue/products", "products"),
  });
  const productIdsKey = (allProducts.data ?? []).map((p) => p.ID).join(",");
  const variantMap = useQuery({
    queryKey: ["variant-map", productIdsKey],
    queryFn: async () => {
      const entries = await Promise.all(
        (allProducts.data ?? []).map(async (p) => {
          const variants = await api.getListField<ProductVariant>(`/catalogue/products/${p.ID}/variants`, "variants");
          return variants.map((v) => [v.ID, { productName: p.Name, sku: v.SKUCode }] as const);
        }),
      );
      return new Map(entries.flat());
    },
    enabled: !!allProducts.data,
  });

  async function searchProducts(q: string) {
    setProductQuery(q);
    setSelectedProduct(null);
    if (q.trim().length < 2) {
      setProductResults([]);
      return;
    }
    const res = await api.getListField<Product>(`/catalogue/products?q=${encodeURIComponent(q)}`, "products");
    setProductResults(res);
  }

  const setPrice = useMutation({
    mutationFn: async () => {
      if (!selectedProduct) throw new Error("Pick a product first.");
      const variants = await api.getListField<ProductVariant>(`/catalogue/products/${selectedProduct.ID}/variants`, "variants");
      const variant = variants[0];
      if (!variant) throw new Error("This product has no variant yet.");
      return api.post(`/pricing/price-lists/${priceList.ID}/items`, {
        product_variant_id: variant.ID,
        unit_id: selectedProduct.BaseUOMID,
        amount,
        currency_code: priceList.CurrencyCode,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["price-list-items", priceList.ID] });
      setProductQuery("");
      setProductResults([]);
      setSelectedProduct(null);
      setAmount("");
    },
  });

  return (
    <div className={layout.panel}>
      <h2>{priceList.Name} prices</h2>
      <div className={ui.formGrid}>
        <div className={ui.field} style={{ gridColumn: "span 2" }}>
          <label htmlFor="pricing-product-search">Product</label>
          {selectedProduct ? (
            <div>
              <strong>{selectedProduct.Name}</strong>{" "}
              <button type="button" className={ui.btnSecondary} onClick={() => setSelectedProduct(null)}>
                Change
              </button>
            </div>
          ) : (
            <>
              <input
                id="pricing-product-search"
                className={ui.input}
                value={productQuery}
                onChange={(e) => void searchProducts(e.target.value)}
                placeholder="Search product…"
              />
              {productResults.length > 0 ? (
                <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
                  {productResults.map((p) => (
                    <li key={p.ID}>
                      <button
                        type="button"
                        className={ui.btnSecondary}
                        style={{ margin: "4px 4px 0 0" }}
                        onClick={() => {
                          setSelectedProduct(p);
                          setProductResults([]);
                        }}
                      >
                        {p.Name}
                      </button>
                    </li>
                  ))}
                </ul>
              ) : null}
            </>
          )}
        </div>
        <div className={ui.field}>
          <label htmlFor="pricing-amount">Price ({priceList.CurrencyCode})</label>
          <input id="pricing-amount" className={ui.input} value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="0.00" />
        </div>
        <button
          type="button"
          className={ui.btnPrimary}
          disabled={!selectedProduct || !amount || setPrice.isPending}
          onClick={() => setPrice.mutate()}
        >
          Save price
        </button>
      </div>
      {setPrice.isError ? (
        <p role="alert" style={{ color: "var(--color-negative)", marginTop: 8 }}>
          {setPrice.error instanceof ApiError ? setPrice.error.message : "Could not save this price."}
        </p>
      ) : null}

      {items.isError ? (
        <p className={layout.errorState} role="alert" style={{ marginTop: 16 }}>
          Couldn't load prices.
        </p>
      ) : items.isPending ? (
        <div className={layout.skeleton} style={{ height: 120, marginTop: 16 }} aria-hidden="true" />
      ) : items.data.length === 0 ? (
        <p className={layout.emptyState} style={{ marginTop: 16 }}>
          No prices set yet — add one above.
        </p>
      ) : (
        <div className={ui.tableScroll} style={{ marginTop: 16 }}>
          <table className={ui.table}>
            <thead>
              <tr>
                <th scope="col">Product</th>
                <th scope="col">SKU</th>
                <th scope="col">Price</th>
              </tr>
            </thead>
            <tbody>
              {items.data.map((it) => {
                const info = variantMap.data?.get(it.ProductVariantID);
                return (
                  <tr key={it.ID}>
                    <td>{info?.productName ?? "—"}</td>
                    <td>{info?.sku ?? "—"}</td>
                    <td className="num">{formatMoney(it.Price)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

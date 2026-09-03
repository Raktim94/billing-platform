import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import ui from "../../components/ui.module.css";
import { api, ApiError } from "../../lib/api-client";
import layout from "../DashboardPage.module.css";

interface Product {
  ID: string;
  Name: string;
  HSNSACCode: string;
  BaseUOMID: string;
}
interface Unit {
  ID: string;
  Code: string;
  Name: string;
}

export function CataloguePage() {
  const queryClient = useQueryClient();
  const [query, setQuery] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState("");
  const [hsn, setHsn] = useState("");
  const [unitId, setUnitId] = useState("");
  const [skuCode, setSkuCode] = useState("");
  const [newUnitCode, setNewUnitCode] = useState("");
  const [newUnitName, setNewUnitName] = useState("");

  const products = useQuery({
    queryKey: ["products", query],
    queryFn: () => api.getListField<Product>(`/catalogue/products${query ? `?q=${encodeURIComponent(query)}` : ""}`, "products"),
  });

  const units = useQuery({
    queryKey: ["units"],
    queryFn: () => api.getListField<Unit>("/catalogue/units", "units"),
  });

  const createUnit = useMutation({
    mutationFn: () => api.post<Unit>("/catalogue/units", { code: newUnitCode, name: newUnitName }),
    onSuccess: (u) => {
      queryClient.invalidateQueries({ queryKey: ["units"] });
      setUnitId(u.ID);
      setNewUnitCode("");
      setNewUnitName("");
    },
  });

  const createProduct = useMutation({
    mutationFn: async () => {
      const product = await api.post<Product>("/catalogue/products", {
        category_id: null,
        brand_id: null,
        base_uom_id: unitId,
        name,
        description: "",
        hsn_sac_code: hsn,
      });
      await api.post(`/catalogue/variants`, {
        product_id: product.ID,
        sku_code: skuCode || product.Name.toUpperCase().replace(/[^A-Z0-9]+/g, "-").slice(0, 24),
        attributes: {},
      });
      return product;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["products"] });
      setName("");
      setHsn("");
      setSkuCode("");
      setShowForm(false);
    },
  });

  return (
    <div className={layout.page}>
      <div className={layout.heading}>
        <div>
          <h1>Catalogue</h1>
          <p className={layout.subtitle}>Products, sold as at least one variant each.</p>
        </div>
        <button type="button" className={ui.btnPrimary} onClick={() => setShowForm((v) => !v)}>
          + New product
        </button>
      </div>

      {showForm ? (
        <div className={layout.panel}>
          {units.data && units.data.length === 0 ? (
            <div className={ui.formGrid} style={{ marginBottom: 16 }}>
              <div className={ui.field}>
                <label htmlFor="new-unit-code">First, add a unit of measure — code (e.g. PCS)</label>
                <input id="new-unit-code" className={ui.input} value={newUnitCode} onChange={(e) => setNewUnitCode(e.target.value)} />
              </div>
              <div className={ui.field}>
                <label htmlFor="new-unit-name">Unit name (e.g. Pieces)</label>
                <input id="new-unit-name" className={ui.input} value={newUnitName} onChange={(e) => setNewUnitName(e.target.value)} />
              </div>
              <button
                type="button"
                className={ui.btnSecondary}
                disabled={!newUnitCode || !newUnitName || createUnit.isPending}
                onClick={() => createUnit.mutate()}
              >
                Add unit
              </button>
            </div>
          ) : null}
          <div className={ui.formGrid}>
            <div className={ui.field}>
              <label htmlFor="product-name">Product name</label>
              <input id="product-name" className={ui.input} value={name} onChange={(e) => setName(e.target.value)} />
            </div>
            <div className={ui.field}>
              <label htmlFor="product-hsn">HSN/SAC code</label>
              <input id="product-hsn" className={ui.input} value={hsn} onChange={(e) => setHsn(e.target.value)} />
            </div>
            <div className={ui.field}>
              <label htmlFor="product-unit">Unit</label>
              <select id="product-unit" className={ui.select} value={unitId} onChange={(e) => setUnitId(e.target.value)}>
                <option value="">Select a unit…</option>
                {units.data?.map((u) => (
                  <option key={u.ID} value={u.ID}>
                    {u.Name} ({u.Code})
                  </option>
                ))}
              </select>
            </div>
            <div className={ui.field}>
              <label htmlFor="product-sku">SKU (optional)</label>
              <input id="product-sku" className={ui.input} value={skuCode} onChange={(e) => setSkuCode(e.target.value)} />
            </div>
          </div>
          <div className={ui.formActions} style={{ marginTop: 12 }}>
            <button
              type="button"
              className={ui.btnPrimary}
              disabled={!name || !unitId || createProduct.isPending}
              onClick={() => createProduct.mutate()}
            >
              Save product
            </button>
          </div>
          {createProduct.isError ? (
            <p role="alert" style={{ color: "var(--color-negative)", marginTop: 8 }}>
              {createProduct.error instanceof ApiError ? createProduct.error.message : "Could not save this product."}
            </p>
          ) : null}
        </div>
      ) : null}

      <div className={layout.panel}>
        <input
          className={ui.input}
          placeholder="Search products…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          style={{ marginBottom: 12, maxWidth: 360 }}
        />
        {products.isError ? (
          <p className={layout.errorState} role="alert">
            Couldn't load products.
          </p>
        ) : products.isPending ? (
          <div className={layout.skeleton} style={{ height: 200 }} aria-hidden="true" />
        ) : products.data.length === 0 ? (
          <p className={layout.emptyState}>No products yet — add your first one above.</p>
        ) : (
          <div className={ui.tableScroll}>
            <table className={ui.table}>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>HSN/SAC</th>
                </tr>
              </thead>
              <tbody>
                {products.data.map((p) => (
                  <tr key={p.ID}>
                    <td>{p.Name}</td>
                    <td>{p.HSNSACCode}</td>
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

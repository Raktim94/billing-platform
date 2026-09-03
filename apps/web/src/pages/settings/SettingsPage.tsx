import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import ui from "../../components/ui.module.css";
import { api, ApiError } from "../../lib/api-client";
import { GST_STATE_CODES } from "../../lib/gstStateCodes";
import { useOrgContext } from "../../lib/useOrgContext";
import layout from "../DashboardPage.module.css";

/** Recovery path for a legal entity that was bootstrapped before it had a
 * state set (docs/adr/0007) — without one, that entity can never finalize
 * a single invoice. Also lets a business add/correct its GSTIN later. */
function GSTDetailsForm({ legalEntityId, currentGSTIN, currentStateCode }: { legalEntityId: string; currentGSTIN: string; currentStateCode: string }) {
  const queryClient = useQueryClient();
  const [gstin, setGstin] = useState(currentGSTIN);
  const [stateCode, setStateCode] = useState(currentStateCode);

  const save = useMutation({
    mutationFn: () => api.put(`/legal-entities/${legalEntityId}/gst`, { gstin, gst_state_code: stateCode }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["legal-entities"] });
    },
  });

  const dirty = gstin !== currentGSTIN || stateCode !== currentStateCode;

  return (
    <div className={ui.formGrid} style={{ marginTop: 12 }}>
      <div className={ui.field}>
        <label htmlFor="settings-gst-state">Business state</label>
        <select id="settings-gst-state" className={ui.select} value={stateCode} onChange={(e) => setStateCode(e.target.value)}>
          <option value="" disabled>
            Select a state…
          </option>
          {GST_STATE_CODES.map((s) => (
            <option key={s.code} value={s.code}>
              {s.name}
            </option>
          ))}
        </select>
        {!currentStateCode ? (
          <p className={ui.muted} style={{ marginTop: 4 }}>
            No state set yet — you cannot finalize any invoice until this is saved.
          </p>
        ) : null}
      </div>
      <div className={ui.field}>
        <label htmlFor="settings-gstin">GSTIN (optional)</label>
        <input id="settings-gstin" className={ui.input} placeholder="Leave blank if not GST-registered" value={gstin} onChange={(e) => setGstin(e.target.value)} />
      </div>
      <button type="button" className={ui.btnPrimary} disabled={!dirty || !stateCode || save.isPending} onClick={() => save.mutate()}>
        {save.isPending ? "Saving…" : "Save GST details"}
      </button>
      {save.isError ? (
        <p role="alert" style={{ color: "var(--color-negative)" }}>
          {save.error instanceof ApiError ? save.error.message : "Could not save GST details."}
        </p>
      ) : null}
      {save.isSuccess ? <p style={{ color: "var(--color-positive)" }}>Saved.</p> : null}
    </div>
  );
}

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
          <>
            <dl style={{ display: "grid", gridTemplateColumns: "180px 1fr", rowGap: 12 }}>
              <dt className={layout.subtitle}>Business</dt>
              <dd>{org.organisation?.Name}</dd>
              <dt className={layout.subtitle}>Legal entity</dt>
              <dd>{org.legalEntity?.LegalName}</dd>
              <dt className={layout.subtitle}>Branch</dt>
              <dd>{org.branch?.Name}</dd>
              <dt className={layout.subtitle}>Warehouse</dt>
              <dd>{org.warehouse?.Name}</dd>
              <dt className={layout.subtitle}>Currency</dt>
              <dd>{org.organisation?.DefaultCurrencyCode}</dd>
            </dl>
            {org.legalEntity ? (
              <GSTDetailsForm
                // Keyed by the values themselves so the form's local
                // draft state remounts fresh after a successful save
                // (queryClient.invalidateQueries refetches org.legalEntity)
                // instead of needing an effect to resync it — avoids the
                // cascading-render pattern an effect-based sync creates.
                key={`${org.legalEntity.ID}-${org.legalEntity.GSTIN}-${org.legalEntity.GSTStateCode}`}
                legalEntityId={org.legalEntity.ID}
                currentGSTIN={org.legalEntity.GSTIN}
                currentStateCode={org.legalEntity.GSTStateCode}
              />
            ) : null}
          </>
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

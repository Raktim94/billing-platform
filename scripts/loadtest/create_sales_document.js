// k6 load test: concurrent "Start sale" (POST /sales/documents), the
// real write path — document-number allocation, RLS-scoped insert — the
// concurrency scenario Stage 11's TODO item ("concurrency scenarios
// re-verified at realistic scale") actually means. Draft creation only
// (not finalize, which additionally posts to the ledger and runs the
// tax engine — a separate, heavier scenario worth its own script if
// this one shows headroom).
//
// Usage:
//   BASE_URL=http://localhost:8099 SESSION_COOKIE=<value> \
//   LEGAL_ENTITY_ID=... BRANCH_ID=... WAREHOUSE_ID=... CUSTOMER_PARTY_ID=... \
//   k6 run scripts/loadtest/create_sales_document.js
import http from "k6/http";
import { check, sleep } from "k6";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const SESSION_COOKIE = __ENV.SESSION_COOKIE;

export const options = {
  scenarios: {
    create_invoice: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "10s", target: 10 },
        { duration: "20s", target: 10 },
        { duration: "10s", target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_duration: ["p(95)<500", "p(99)<1500"],
    http_req_failed: ["rate<0.01"],
  },
};

export default function () {
  const payload = JSON.stringify({
    legal_entity_id: __ENV.LEGAL_ENTITY_ID,
    branch_id: __ENV.BRANCH_ID,
    warehouse_id: __ENV.WAREHOUSE_ID,
    customer_party_id: __ENV.CUSTOMER_PARTY_ID,
    document_type: "TAX_INVOICE",
    place_of_supply_state_code: "27",
    currency_code: "INR",
    base_currency_code: "INR",
    pricing_mode: "EXCLUSIVE",
  });
  const res = http.post(`${BASE_URL}/api/v1/sales/documents`, payload, {
    headers: {
      "Content-Type": "application/json",
      Cookie: `bp_session=${SESSION_COOKIE}`,
    },
  });
  check(res, {
    "status is 201": (r) => r.status === 201,
    "has a document number": (r) => {
      try {
        return !!JSON.parse(r.body).document_number;
      } catch {
        return false;
      }
    },
  });
  sleep(0.1);
}

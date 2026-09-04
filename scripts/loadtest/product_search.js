// k6 load test: concurrent product search (GET /catalogue/products?q=...),
// the actual billing-counter search path (brief §24/§25's "must feel
// instantaneous"), against a realistically large catalogue.
//
// Usage:
//   BASE_URL=http://localhost:8099 SESSION_COOKIE=<value> k6 run scripts/loadtest/product_search.js
//
// SESSION_COOKIE is the bp_session cookie value from a real logged-in
// session (see docs/operations/deployment.md or just POST /auth/login
// locally and read the Set-Cookie header) — there is no load-test-only
// auth bypass, this exercises the real authenticated path.
import http from "k6/http";
import { check, sleep } from "k6";

const BASE_URL = __ENV.BASE_URL || "http://localhost:8080";
const SESSION_COOKIE = __ENV.SESSION_COOKIE;

const queries = ["Steel Bolt", "Copper Cable", "Plastic Panel", "Rubber Gasket", "Glass Sensor"];

export const options = {
  scenarios: {
    search: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "10s", target: 20 },
        { duration: "20s", target: 20 },
        { duration: "10s", target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_duration: ["p(95)<300", "p(99)<800"],
    http_req_failed: ["rate<0.01"],
  },
};

export default function () {
  const q = queries[Math.floor(Math.random() * queries.length)];
  const res = http.get(`${BASE_URL}/api/v1/catalogue/products?q=${encodeURIComponent(q)}`, {
    headers: { Cookie: `bp_session=${SESSION_COOKIE}` },
  });
  check(res, {
    "status is 200": (r) => r.status === 200,
    "returns at most 20 results (search cap)": (r) => {
      try {
        // `products` is `null`, not `[]`, when a query matches nothing
        // (Go nil-slice-to-JSON — see docs/TODO.md Stage 11 note) — a
        // real but separate, systemic (38 call sites) minor API-shape
        // finding, not something this load test should fail on.
        const products = JSON.parse(r.body).products;
        return products === null || products.length <= 20;
      } catch {
        return false;
      }
    },
  });
  sleep(0.1);
}

import { useQuery } from "@tanstack/react-query";
import { api } from "./api-client";

/** Mirrors internal/modules/organisation/domain.LegalEntity/Branch/Warehouse
 * (no json tags, so Go's exported field names serialize verbatim). */
export interface LegalEntity {
  ID: string;
  LegalName: string;
  GSTIN: string;
  GSTStateCode: string;
}
export interface Branch {
  ID: string;
  LegalEntityID: string;
  Code: string;
  Name: string;
}
export interface Warehouse {
  ID: string;
  BranchID: string;
  Code: string;
  Name: string;
}
export interface Organisation {
  ID: string;
  Name: string;
  DefaultCurrencyCode: string;
  EWayBillMode: "FREE_PORTAL" | "AUTOMATIC_API";
}

/** Every screen that creates a document (Sales, Purchases, ...) needs a
 * legal entity / branch / warehouse to post against. Almost every
 * self-hosted install of this size has exactly one of each (single-branch
 * small business) — this hook resolves "the" one, so screens don't each
 * re-implement a picker for something that, for the overwhelming majority
 * of installs, is not actually a choice. A business that genuinely has
 * multiple branches/warehouses can still see and change them via
 * Settings (existing legal-entities/branches/warehouses endpoints); nothing
 * here prevents that, it just isn't this pass's UI.
 */
export function useOrgContext() {
  const organisation = useQuery({
    queryKey: ["organisation"],
    queryFn: () => api.get<Organisation>("/organisation"),
  });
  const legalEntities = useQuery({
    queryKey: ["legal-entities"],
    queryFn: () => api.get<{ legal_entities: LegalEntity[] }>("/legal-entities"),
  });
  const branches = useQuery({
    queryKey: ["branches"],
    queryFn: () => api.get<{ branches: Branch[] }>("/branches"),
  });
  const firstBranch = branches.data?.branches[0];
  const warehouses = useQuery({
    queryKey: ["warehouses", firstBranch?.ID],
    queryFn: () => api.get<{ warehouses: Warehouse[] }>(`/branches/${firstBranch?.ID}/warehouses`),
    enabled: !!firstBranch,
  });

  const isPending = organisation.isPending || legalEntities.isPending || branches.isPending || warehouses.isPending;
  const isError = organisation.isError || legalEntities.isError || branches.isError || warehouses.isError;

  return {
    isPending,
    isError,
    organisation: organisation.data,
    legalEntity: legalEntities.data?.legal_entities[0],
    branch: firstBranch,
    warehouse: warehouses.data?.warehouses[0],
  };
}

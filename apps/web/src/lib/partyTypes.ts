/** Mirrors internal/modules/contacts/domain.Party (no json tags — Go's
 * exported field names serialize verbatim). */
export type PartyType = "CUSTOMER" | "SUPPLIER" | "BOTH";

export interface Party {
  ID: string;
  PartyType: PartyType;
  LegalName: string;
  TradeName: string;
  Phone: string;
  Email: string;
  CurrencyCode: string;
  CreditLimitAmount: string | null;
  PaymentTermsDays: number | null;
  Notes: string;
  Status: "ACTIVE" | "INACTIVE";
}

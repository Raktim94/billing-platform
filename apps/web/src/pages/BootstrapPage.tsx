import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useNavigate } from "@tanstack/react-router";
import styles from "./auth.module.css";
import { api, ApiError } from "../lib/api-client";

/**
 * First-run setup: creates the first organisation + owner user via
 * internal/modules/identity's POST /auth/bootstrap (Stage 2). That
 * endpoint has no permission check by design — it's meant for a fresh,
 * empty deployment only, and the composition root gates it behind
 * ENABLE_BOOTSTRAP (see deploy/compose/.env.example). This screen doesn't
 * try to detect whether bootstrap is still open; a deployment that has
 * disabled it will simply 404/error here, which is an acceptable outcome
 * for a screen nobody should be reaching post-setup anyway.
 */
const schema = z
  .object({
    organisationName: z.string().min(1, "Business name is required"),
    legalEntityName: z.string().min(1, "Legal entity name is required"),
    branchName: z.string().min(1, "Branch name is required"),
    warehouseName: z.string().min(1, "Warehouse name is required"),
    ownerFullName: z.string().min(1, "Your name is required"),
    ownerEmail: z.string().min(1, "Email is required").email("Enter a valid email address"),
    ownerPassword: z.string().min(12, "Use at least 12 characters"),
    confirmPassword: z.string().min(1, "Confirm your password"),
  })
  .refine((v) => v.ownerPassword === v.confirmPassword, {
    message: "Passwords do not match",
    path: ["confirmPassword"],
  });
type FormValues = z.infer<typeof schema>;

interface BootstrapResponse {
  organisation_id: string;
}

export function BootstrapPage() {
  const navigate = useNavigate();
  const [serverError, setServerError] = useState<string | null>(null);
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({ resolver: zodResolver(schema) });

  const onSubmit = async (values: FormValues) => {
    setServerError(null);
    try {
      await api.post<BootstrapResponse>("/auth/bootstrap", {
        organisation_name: values.organisationName,
        legal_entity_name: values.legalEntityName,
        branch_name: values.branchName,
        warehouse_name: values.warehouseName,
        owner_full_name: values.ownerFullName,
        owner_email: values.ownerEmail,
        owner_password: values.ownerPassword,
      });
      await navigate({ to: "/login" });
    } catch (err) {
      setServerError(err instanceof ApiError ? err.message : "Could not complete setup. Please try again.");
    }
  };

  return (
    <div className={styles.page}>
      <div className={styles.card} style={{ maxWidth: 460 }}>
        <div className={styles.wordmark}>
          <span className={styles.mark} aria-hidden="true" />
          billing-platform
        </div>
        <h1 className={styles.title}>Set up your business</h1>
        <p className={styles.subtitle}>This runs once, on a brand-new installation.</p>

        {serverError ? (
          <div className={styles.formError} role="alert">
            {serverError}
          </div>
        ) : null}

        {/* eslint-disable-next-line @typescript-eslint/no-misused-promises */}
        <form onSubmit={handleSubmit(onSubmit)} noValidate>
          <div className={styles.field}>
            <label htmlFor="organisationName">Business name</label>
            <input id="organisationName" {...register("organisationName")} />
            {errors.organisationName ? <p className={styles.error}>{errors.organisationName.message}</p> : null}
          </div>
          <div className={styles.field}>
            <label htmlFor="legalEntityName">Legal entity name</label>
            <input id="legalEntityName" {...register("legalEntityName")} />
            {errors.legalEntityName ? <p className={styles.error}>{errors.legalEntityName.message}</p> : null}
          </div>
          <div className={styles.grid2}>
            <div className={styles.field}>
              <label htmlFor="branchName">First branch</label>
              <input id="branchName" {...register("branchName")} />
              {errors.branchName ? <p className={styles.error}>{errors.branchName.message}</p> : null}
            </div>
            <div className={styles.field}>
              <label htmlFor="warehouseName">First warehouse</label>
              <input id="warehouseName" {...register("warehouseName")} />
              {errors.warehouseName ? <p className={styles.error}>{errors.warehouseName.message}</p> : null}
            </div>
          </div>
          <div className={styles.field}>
            <label htmlFor="ownerFullName">Your name</label>
            <input id="ownerFullName" autoComplete="name" {...register("ownerFullName")} />
            {errors.ownerFullName ? <p className={styles.error}>{errors.ownerFullName.message}</p> : null}
          </div>
          <div className={styles.field}>
            <label htmlFor="ownerEmail">Your email</label>
            <input id="ownerEmail" type="email" autoComplete="username" {...register("ownerEmail")} />
            {errors.ownerEmail ? <p className={styles.error}>{errors.ownerEmail.message}</p> : null}
          </div>
          <div className={styles.grid2}>
            <div className={styles.field}>
              <label htmlFor="ownerPassword">Password</label>
              <input id="ownerPassword" type="password" autoComplete="new-password" {...register("ownerPassword")} />
              {errors.ownerPassword ? <p className={styles.error}>{errors.ownerPassword.message}</p> : null}
            </div>
            <div className={styles.field}>
              <label htmlFor="confirmPassword">Confirm password</label>
              <input
                id="confirmPassword"
                type="password"
                autoComplete="new-password"
                {...register("confirmPassword")}
              />
              {errors.confirmPassword ? <p className={styles.error}>{errors.confirmPassword.message}</p> : null}
            </div>
          </div>
          <button type="submit" className={styles.submit} disabled={isSubmitting}>
            {isSubmitting ? "Creating your business…" : "Create business and continue"}
          </button>
        </form>
      </div>
    </div>
  );
}

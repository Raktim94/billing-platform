import { createContext, use, useCallback, useEffect, useState, type ReactNode } from "react";
import { api, AUTH_EVENT, ApiError } from "../lib/api-client";
import { clearSessionHint, readSessionHint, writeSessionHint, type SessionHint } from "./session";

interface LoginResponse {
  organisation_id: string;
  user_id: string;
  idle_expires_at: string;
  absolute_expires_at: string;
}

interface AuthContextValue {
  session: SessionHint | null;
  login: (email: string, password: string, mfaCode?: string) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<SessionHint | null>(() => readSessionHint());

  useEffect(() => {
    const onUnauthorized = () => {
      clearSessionHint();
      setSession(null);
    };
    window.addEventListener(AUTH_EVENT, onUnauthorized);
    return () => window.removeEventListener(AUTH_EVENT, onUnauthorized);
  }, []);

  const login = useCallback(async (email: string, password: string, mfaCode?: string) => {
    const res = await api.post<LoginResponse>("/auth/login", {
      email,
      password,
      mfa_code: mfaCode ?? "",
    });
    const hint: SessionHint = { organisationId: res.organisation_id, userId: res.user_id };
    writeSessionHint(hint);
    setSession(hint);
  }, []);

  const logout = useCallback(async () => {
    try {
      await api.post("/auth/logout");
    } catch {
      // Best-effort — a network failure or an already-expired session
      // shouldn't block the user from clearing local state and landing
      // back on the login screen.
    }
    clearSessionHint();
    setSession(null);
  }, []);

  return <AuthContext value={{ session, login, logout }}>{children}</AuthContext>;
}

export function useAuth(): AuthContextValue {
  const ctx = use(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}

export function isUnauthorized(err: unknown): boolean {
  return err instanceof ApiError && err.status === 401;
}

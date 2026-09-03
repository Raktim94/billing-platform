import { useEffect, useRef, useState, type ReactNode } from "react";
import { Link, useRouterState } from "@tanstack/react-router";
import styles from "./AppShell.module.css";
import { useAuth } from "../auth/AuthProvider";
import { useTheme } from "../theme/ThemeProvider";

const NAV_ITEMS = [
  { to: "/", label: "Dashboard" },
  { to: "/sales", label: "Sales" },
  { to: "/purchases", label: "Purchases" },
  { to: "/inventory", label: "Inventory" },
  { to: "/contacts", label: "Contacts" },
  { to: "/accounting", label: "Accounting" },
  { to: "/gst", label: "GST / Tax" },
  { to: "/reports", label: "Reports" },
  { to: "/integrations", label: "Integrations" },
  { to: "/settings", label: "Settings" },
] as const;

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const { logout } = useAuth();
  const { theme, toggle } = useTheme();
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menuOpen) return;
    const onClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [menuOpen]);

  return (
    <div className={styles.shell}>
      <nav className={styles.nav} aria-label="Primary">
        <div className={styles.wordmark}>
          <span className={styles.mark} aria-hidden="true" />
          billing-platform
        </div>
        <ul className={styles.navList}>
          {NAV_ITEMS.map((item) => (
            <li key={item.to}>
              <Link
                to={item.to}
                className={`${styles.navLink} ${pathname === item.to ? styles.navLinkActive : ""}`.trim()}
              >
                {item.label}
              </Link>
            </li>
          ))}
        </ul>
      </nav>

      <header className={styles.topbar}>
        <div className={styles.search} role="search">
          <span aria-hidden="true">⌕</span>
          <input
            type="search"
            placeholder="Search invoices, customers, products…"
            aria-label="Global search"
          />
        </div>
        <div className={styles.topbarSpacer} />
        <button type="button" className={styles.quickCreate}>
          + Quick create
        </button>
        <button
          type="button"
          className={styles.iconButton}
          onClick={toggle}
          aria-label={theme === "dark" ? "Switch to light mode" : "Switch to dark mode"}
        >
          {theme === "dark" ? "☀" : "☾"}
        </button>
        <div className={styles.userMenu} ref={menuRef}>
          <button
            type="button"
            className={styles.userButton}
            onClick={() => setMenuOpen((v) => !v)}
            aria-expanded={menuOpen}
            aria-haspopup="menu"
          >
            <span className={styles.avatar} aria-hidden="true">
              U
            </span>
          </button>
          {menuOpen ? (
            <div className={styles.userDropdown} role="menu">
              <Link to="/settings" role="menuitem" onClick={() => setMenuOpen(false)}>
                Settings
              </Link>
              <button type="button" role="menuitem" onClick={() => void logout()}>
                Log out
              </button>
            </div>
          ) : null}
        </div>
      </header>

      <main className={styles.main}>{children}</main>
    </div>
  );
}

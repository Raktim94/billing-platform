import styles from "./DashboardPage.module.css";

/** Every primary-nav section beyond Dashboard is a placeholder in this
 * pass (Stage 10b-1 — frontend foundation only); each backend module
 * (sales, inventory, accounting, ...) already exists and is fully tested,
 * the screens themselves are later passes. */
export function PlaceholderPage({ title }: { title: string }) {
  return (
    <div className={styles.page}>
      <div className={styles.heading}>
        <div>
          <h1>{title}</h1>
          <p className={styles.subtitle}>This section is coming in a later build pass.</p>
        </div>
      </div>
      <div className={styles.panel}>
        <p className={styles.emptyState}>
          The {title.toLowerCase()} API is already built and tested — this screen just hasn't been wired up yet.
        </p>
      </div>
    </div>
  );
}

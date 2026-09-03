import styles from "./StatCard.module.css";

type Polarity = "neutral" | "positive" | "negative" | "warning";

export function StatCard({
  label,
  value,
  polarity = "neutral",
}: {
  label: string;
  value: string;
  polarity?: Polarity;
}) {
  return (
    <div className={styles.card} data-polarity={polarity}>
      <span className={styles.label}>{label}</span>
      <span className={styles.value} data-polarity={polarity}>
        {value}
      </span>
    </div>
  );
}

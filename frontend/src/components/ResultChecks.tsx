import { rowId } from "../form";
import type { CheckRow } from "../form";
export function ResultChecks({
  rows,
  onChange,
  disabled,
}: {
  rows: CheckRow[];
  onChange: (rows: CheckRow[]) => void;
  disabled: boolean;
}) {
  const update = (id: string, change: Partial<CheckRow>) =>
    onChange(rows.map((r) => (r.id === id ? { ...r, ...change } : r)));
  return (
    <>
      <p className="help">
        For a response like <code>{'{"notificationCount": 1}'}</code>, check{" "}
        <code>/notificationCount</code> against the number <code>1</code>. A
        blank path checks the whole response.
      </p>
      {rows.map((r, i) => (
        <div className="check-row" key={r.id}>
          <label>
            Field path
            <input
              aria-label={`Check ${i + 1} field path`}
              placeholder="/notificationCount"
              value={r.pointer}
              disabled={disabled}
              onChange={(e) => update(r.id, { pointer: e.target.value })}
            />
          </label>
          <label>
            Value type
            <select
              aria-label={`Check ${i + 1} value type`}
              value={r.type}
              disabled={disabled}
              onChange={(e) =>
                update(r.id, {
                  type: e.target.value as CheckRow["type"],
                  value: e.target.value === "boolean" ? "true" : "",
                })
              }
            >
              <option value="number">Number</option>
              <option value="string">Text</option>
              <option value="boolean">True / false</option>
              <option value="null">Null</option>
            </select>
          </label>
          <label>
            Expected value
            {r.type === "boolean" ? (
              <select
                aria-label={`Check ${i + 1} expected value`}
                value={r.value}
                disabled={disabled}
                onChange={(e) => update(r.id, { value: e.target.value })}
              >
                <option value="true">true</option>
                <option value="false">false</option>
              </select>
            ) : (
              <input
                aria-label={`Check ${i + 1} expected value`}
                placeholder={
                  r.type === "number"
                    ? "1"
                    : r.type === "null"
                      ? "null"
                      : "paid"
                }
                value={r.value}
                disabled={disabled || r.type === "null"}
                onChange={(e) => update(r.id, { value: e.target.value })}
              />
            )}
          </label>
          <button
            type="button"
            className="quiet remove"
            aria-label={`Remove check ${i + 1}`}
            disabled={disabled || rows.length === 1}
            onClick={() => onChange(rows.filter((c) => c.id !== r.id))}
          >
            Remove
          </button>
        </div>
      ))}
      <button
        type="button"
        className="quiet"
        disabled={disabled || rows.length >= 20}
        onClick={() =>
          onChange([
            ...rows,
            { id: rowId(), pointer: "", value: "", type: "number" },
          ])
        }
      >
        Add result check
      </button>
    </>
  );
}

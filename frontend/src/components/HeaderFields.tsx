import { rowId } from "../form";
import type { HeaderRow } from "../form";
export function HeaderFields({
  rows,
  onChange,
  label,
  disabled,
}: {
  rows: HeaderRow[];
  onChange: (rows: HeaderRow[]) => void;
  label: string;
  disabled: boolean;
}) {
  return (
    <div className="header-fields">
      {rows.map((row, i) => (
        <div className="header-row" key={row.id}>
          <label>
            Header name
            <input
              aria-label={`${label} header ${i + 1} name`}
              value={row.name}
              placeholder="Authorization"
              disabled={disabled}
              onChange={(e) =>
                onChange(
                  rows.map((r) =>
                    r.id === row.id ? { ...r, name: e.target.value } : r,
                  ),
                )
              }
            />
          </label>
          <label>
            Header value
            <input
              aria-label={`${label} header ${i + 1} value`}
              value={row.value}
              placeholder="Bearer your-test-token"
              autoComplete="off"
              disabled={disabled}
              onChange={(e) =>
                onChange(
                  rows.map((r) =>
                    r.id === row.id ? { ...r, value: e.target.value } : r,
                  ),
                )
              }
            />
          </label>
          <button
            type="button"
            className="quiet remove"
            aria-label={`Remove ${label.toLowerCase()} header ${i + 1}`}
            disabled={disabled}
            onClick={() => onChange(rows.filter((r) => r.id !== row.id))}
          >
            Remove
          </button>
        </div>
      ))}
      <button
        type="button"
        className="quiet"
        disabled={disabled || rows.length >= 32}
        onClick={() =>
          onChange([...rows, { id: rowId(), name: "", value: "" }])
        }
      >
        Add {label.toLowerCase()} header
      </button>
    </div>
  );
}

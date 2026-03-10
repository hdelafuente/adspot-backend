"use client";

import {
  useEffect,
  useRef,
  useState,
  useMemo,
  useCallback,
} from "react";
import { toast } from "sonner";
import { getAllAdSpots, deactivateAdSpot } from "@/lib/api";
import { type AdSpot, type Placement, type Status, PLACEMENTS } from "@/lib/types";

// ── Constants ─────────────────────────────────────────────────────────────────

const PLACEMENT_COLORS: Record<Placement, string> = {
  home_screen: "bg-blue-100 text-blue-700",
  ride_summary: "bg-purple-100 text-purple-700",
  map_view: "bg-teal-100 text-teal-700",
};

const PLACEMENT_LABELS: Record<Placement, string> = {
  home_screen: "Home Screen",
  ride_summary: "Ride Summary",
  map_view: "Map View",
};

// ── Sub-components ────────────────────────────────────────────────────────────

function PlacementBadge({ placement }: { placement: Placement }) {
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${PLACEMENT_COLORS[placement]}`}
    >
      {PLACEMENT_LABELS[placement]}
    </span>
  );
}

function StatusBadge({
  status,
  optimistic,
}: {
  status: AdSpot["status"];
  optimistic?: boolean;
}) {
  return (
    <span
      className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium transition-opacity ${
        status === "active"
          ? "bg-green-100 text-green-700"
          : "bg-red-100 text-red-600"
      } ${optimistic ? "opacity-60" : ""}`}
    >
      <span
        className={`w-1.5 h-1.5 rounded-full ${
          status === "active" ? "bg-green-500" : "bg-red-400"
        } ${optimistic ? "animate-pulse" : ""}`}
      />
      {status === "active" ? "Activo" : "Inactivo"}
      {optimistic && <span className="ml-0.5 text-[10px] opacity-70">(guardando…)</span>}
    </span>
  );
}

function TableSkeleton() {
  return (
    <div className="animate-pulse space-y-3">
      {[...Array(4)].map((_, i) => (
        <div key={i} className="h-12 bg-gray-100 rounded" />
      ))}
    </div>
  );
}

// ── Placement multiselect dropdown ────────────────────────────────────────────

function PlacementMultiselect({
  selected,
  onChange,
}: {
  selected: Set<Placement>;
  onChange: (next: Set<Placement>) => void;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  // Close on outside click
  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  function toggle(p: Placement) {
    const next = new Set(selected);
    if (next.has(p)) next.delete(p);
    else next.add(p);
    onChange(next);
  }

  const label =
    selected.size === 0
      ? "Placement"
      : selected.size === PLACEMENTS.length
        ? "Todos"
        : [...selected].map((p) => PLACEMENT_LABELS[p]).join(", ");

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className={`inline-flex items-center gap-2 text-sm border rounded-md px-3 py-1.5 bg-white transition-colors focus:outline-none focus:ring-2 focus:ring-indigo-500 ${
          selected.size > 0
            ? "border-indigo-400 text-indigo-700"
            : "border-gray-300 text-gray-600 hover:border-gray-400"
        }`}
      >
        <span className="max-w-[180px] truncate">{label}</span>
        {selected.size > 0 && (
          <span className="inline-flex items-center justify-center w-4 h-4 text-[10px] font-bold rounded-full bg-indigo-100 text-indigo-700">
            {selected.size}
          </span>
        )}
        <svg
          className={`w-3.5 h-3.5 opacity-50 transition-transform ${open ? "rotate-180" : ""}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {open && (
        <div className="absolute z-20 mt-1 w-52 bg-white border border-gray-200 rounded-lg shadow-lg py-1">
          {/* Select all / Clear */}
          <div className="flex items-center justify-between px-3 py-1.5 border-b border-gray-100">
            <button
              className="text-xs text-indigo-600 hover:underline"
              onClick={() => onChange(new Set(PLACEMENTS.map((p) => p.value)))}
            >
              Todos
            </button>
            <button
              className="text-xs text-gray-400 hover:text-gray-600 hover:underline"
              onClick={() => onChange(new Set())}
            >
              Limpiar
            </button>
          </div>

          {PLACEMENTS.map((p) => (
            <label
              key={p.value}
              className="flex items-center gap-2.5 px-3 py-2 cursor-pointer hover:bg-gray-50 text-sm text-gray-700"
            >
              <input
                type="checkbox"
                checked={selected.has(p.value)}
                onChange={() => toggle(p.value)}
                className="w-3.5 h-3.5 rounded border-gray-300 text-indigo-600 focus:ring-indigo-500"
              />
              <PlacementBadge placement={p.value} />
            </label>
          ))}
        </div>
      )}
    </div>
  );
}

// ── Status segmented control ──────────────────────────────────────────────────

type StatusFilter = "all" | Status;

function StatusSegment({
  value,
  onChange,
}: {
  value: StatusFilter;
  onChange: (v: StatusFilter) => void;
}) {
  const options: { key: StatusFilter; label: string }[] = [
    { key: "all", label: "Todos" },
    { key: "active", label: "Activo" },
    { key: "inactive", label: "Inactivo" },
  ];

  return (
    <div className="inline-flex rounded-md border border-gray-300 overflow-hidden text-sm">
      {options.map(({ key, label }) => (
        <button
          key={key}
          type="button"
          onClick={() => onChange(key)}
          className={`px-3 py-1.5 transition-colors ${
            value === key
              ? "bg-indigo-600 text-white font-medium"
              : "bg-white text-gray-600 hover:bg-gray-50"
          } ${key !== "all" ? "border-l border-gray-300" : ""}`}
        >
          {label}
        </button>
      ))}
    </div>
  );
}

// ── Main component ────────────────────────────────────────────────────────────

export default function AdSpotTable() {
  const [adspots, setAdspots] = useState<AdSpot[]>([]);
  const [loading, setLoading] = useState(true);

  // Filters
  const [titleSearch, setTitleSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [placementFilters, setPlacementFilters] = useState<Set<Placement>>(new Set());

  // Tracks which IDs are in-flight (optimistic but not yet confirmed)
  const [pendingIds, setPendingIds] = useState<Set<string>>(new Set());

  useEffect(() => {
    setLoading(true);
    getAllAdSpots()
      .then(setAdspots)
      .catch((err: Error) => toast.error(`Error al cargar: ${err.message}`))
      .finally(() => setLoading(false));
  }, []);

  // ── Optimistic deactivate ─────────────────────────────────────────────────

  const handleDeactivate = useCallback(
    (id: string, title: string) => {
      // 1. Snapshot for rollback
      const snapshot = adspots;

      // 2. Immediately apply optimistic update
      setAdspots((prev) =>
        prev.map((s) =>
          s.id === id
            ? { ...s, status: "inactive" as const, deactivatedAt: new Date().toISOString() }
            : s
        )
      );
      setPendingIds((prev) => new Set(prev).add(id));

      // 3. Fire API call — use toast.promise so the toast reflects the real outcome
      toast.promise(
        deactivateAdSpot(id)
          .then((updated) => {
            // Confirm with real server data
            setAdspots((prev) => prev.map((s) => (s.id === id ? updated : s)));
          })
          .catch((err) => {
            // Rollback on error
            setAdspots(snapshot);
            throw err;
          })
          .finally(() => {
            setPendingIds((prev) => {
              const next = new Set(prev);
              next.delete(id);
              return next;
            });
          }),
        {
          loading: `Desactivando "${title}"…`,
          success: `"${title}" desactivado correctamente`,
          error: (err: unknown) =>
            err instanceof Error ? err.message : "Error al desactivar",
        }
      );
    },
    [adspots]
  );

  // ── Client-side filtering ─────────────────────────────────────────────────

  const filtered = useMemo(() => {
    const q = titleSearch.trim().toLowerCase();
    return adspots.filter((s) => {
      const matchesTitle = q === "" || s.title.toLowerCase().includes(q);
      const matchesStatus = statusFilter === "all" || s.status === statusFilter;
      const matchesPlacement =
        placementFilters.size === 0 || placementFilters.has(s.placement);
      return matchesTitle && matchesStatus && matchesPlacement;
    });
  }, [adspots, titleSearch, statusFilter, placementFilters]);

  const hasFilters =
    titleSearch !== "" || statusFilter !== "all" || placementFilters.size > 0;

  function clearFilters() {
    setTitleSearch("");
    setStatusFilter("all");
    setPlacementFilters(new Set());
  }

  // ── Render ────────────────────────────────────────────────────────────────

  return (
    <div className="space-y-4">
      {/* Filter bar */}
      <div className="flex flex-wrap items-center gap-3">
        {/* Title search */}
        <div className="relative">
          <svg
            className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-gray-400 pointer-events-none"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2.5}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M21 21l-4.35-4.35m0 0A7.5 7.5 0 104.5 4.5a7.5 7.5 0 0012.15 12.15z"
            />
          </svg>
          <input
            type="text"
            placeholder="Buscar por título…"
            value={titleSearch}
            onChange={(e) => setTitleSearch(e.target.value)}
            className="pl-8 pr-3 py-1.5 text-sm border border-gray-300 rounded-md bg-white w-52 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500"
          />
        </div>

        {/* Status filter */}
        <StatusSegment value={statusFilter} onChange={setStatusFilter} />

        {/* Placement multiselect */}
        <PlacementMultiselect
          selected={placementFilters}
          onChange={setPlacementFilters}
        />

        {/* Clear filters */}
        {hasFilters && (
          <button
            onClick={clearFilters}
            className="text-xs text-gray-400 hover:text-gray-600 underline underline-offset-2 transition-colors"
          >
            Limpiar filtros
          </button>
        )}

        {/* Result count */}
        <span className="ml-auto text-sm text-gray-400">
          {filtered.length} resultado{filtered.length !== 1 ? "s" : ""}
        </span>
      </div>

      {/* Table */}
      {loading ? (
        <TableSkeleton />
      ) : filtered.length === 0 ? (
        <div className="text-center py-16 text-gray-400">
          <p className="text-4xl mb-3">{hasFilters ? "🔍" : "📭"}</p>
          <p className="text-sm">
            {hasFilters
              ? "Ningún Ad Spot coincide con los filtros aplicados"
              : "No hay Ad Spots para mostrar"}
          </p>
          {hasFilters && (
            <button
              onClick={clearFilters}
              className="mt-3 text-xs text-indigo-600 hover:underline"
            >
              Limpiar filtros
            </button>
          )}
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-gray-200 bg-white shadow-sm">
          <table className="min-w-full divide-y divide-gray-200 text-sm">
            <thead className="bg-gray-50">
              <tr>
                {["Título", "Imagen", "Placement", "Estado", "TTL (min)", "Creado", ""].map(
                  (h) => (
                    <th
                      key={h}
                      className="px-4 py-3 text-left text-xs font-semibold text-gray-500 uppercase tracking-wide"
                    >
                      {h}
                    </th>
                  )
                )}
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100">
              {filtered.map((spot) => {
                const isPending = pendingIds.has(spot.id);
                return (
                  <tr
                    key={spot.id}
                    className={`transition-all duration-300 ${
                      isPending
                        ? "bg-orange-50"
                        : spot.status === "inactive"
                          ? "bg-gray-50 opacity-70"
                          : "hover:bg-gray-50"
                    }`}
                  >
                    {/* Title */}
                    <td className="px-4 py-3 font-medium text-gray-900 max-w-[200px] truncate">
                      {spot.title}
                    </td>

                    {/* Image thumbnail */}
                    <td className="px-4 py-3">
                      <img
                        src={spot.imageUrl}
                        alt={spot.title}
                        className="h-9 w-14 object-cover rounded border border-gray-200"
                        onError={(e) => {
                          (e.target as HTMLImageElement).src =
                            "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='56' height='36'%3E%3Crect width='56' height='36' fill='%23e5e7eb'/%3E%3Ctext x='50%25' y='50%25' dominant-baseline='middle' text-anchor='middle' font-size='10' fill='%239ca3af'%3EN/A%3C/text%3E%3C/svg%3E";
                        }}
                      />
                    </td>

                    {/* Placement */}
                    <td className="px-4 py-3">
                      <PlacementBadge placement={spot.placement} />
                    </td>

                    {/* Status */}
                    <td className="px-4 py-3">
                      <StatusBadge status={spot.status} optimistic={isPending} />
                    </td>

                    {/* TTL */}
                    <td className="px-4 py-3 text-gray-500">
                      {spot.ttlMinutes ?? <span className="text-gray-300">—</span>}
                    </td>

                    {/* Created At */}
                    <td className="px-4 py-3 text-gray-500 whitespace-nowrap">
                      {new Date(spot.createdAt).toLocaleString("es-AR", {
                        dateStyle: "short",
                        timeStyle: "short",
                      })}
                    </td>

                    {/* Actions */}
                    <td className="px-4 py-3 text-right">
                      {spot.status === "active" && !isPending && (
                        <button
                          onClick={() => handleDeactivate(spot.id, spot.title)}
                          className="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium rounded-md border border-red-200 text-red-600 hover:bg-red-50 transition-colors"
                        >
                          Desactivar
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

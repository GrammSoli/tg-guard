import { create } from "zustand";
import type { Subscription } from "@/types/subscription";
import type { ServiceCategory } from "@/lib/mockData";
import { api } from "@/lib/api";

/** How to order the merged subscriptions/rooms list. */
export type SortBy = "nextPayment" | "priceDesc" | "priceAsc" | "alphabetical";

/** Which item kinds are visible. Empty set = show ALL kinds (no-op filter). */
export type FilterType = "subscription" | "room";

export const DEFAULT_SORT: SortBy = "nextPayment";

interface SubscriptionStore {
  items: Subscription[];
  loading: boolean;
  error: string | null;
  filter: string;

  // Advanced filter state — drives FilterSheet + Dashboard's derived list.
  sortBy: SortBy;
  filterTypes: FilterType[];
  filterCategories: ServiceCategory[];

  setFilter: (q: string) => void;
  setSortBy: (s: SortBy) => void;
  setFilterTypes: (t: FilterType[]) => void;
  setFilterCategories: (c: ServiceCategory[]) => void;
  /** Restore the "no filters applied" state. Does NOT touch the search input. */
  resetFilters: () => void;
  /** True iff any non-default filter is currently active. Drives the indicator dot on the filter button. */
  hasActiveFilters: () => boolean;

  fetchSubscriptions: () => Promise<void>;
  addSubscription: (s: Omit<Subscription, "id">) => Promise<void>;
  updateSubscription: (id: string, data: Partial<Subscription>) => Promise<void>;
  deleteSubscription: (id: string) => Promise<void>;
  setItems: (items: Subscription[]) => void;
}

export const useSubscriptionStore = create<SubscriptionStore>((set, get) => ({
  items: [],
  loading: false,
  error: null,
  filter: "",

  sortBy: DEFAULT_SORT,
  filterTypes: [],
  filterCategories: [],

  setFilter: (q) => set({ filter: q }),
  setSortBy: (s) => set({ sortBy: s }),
  setFilterTypes: (t) => set({ filterTypes: t }),
  setFilterCategories: (c) => set({ filterCategories: c }),
  resetFilters: () =>
    set({
      sortBy: DEFAULT_SORT,
      filterTypes: [],
      filterCategories: [],
    }),
  hasActiveFilters: () => {
    const s = get();
    return (
      s.sortBy !== DEFAULT_SORT ||
      s.filterTypes.length > 0 ||
      s.filterCategories.length > 0
    );
  },

  fetchSubscriptions: async () => {
    set({ loading: true, error: null });
    try {
      const items = await api<Subscription[]>("/subscriptions");
      set({ items, loading: false });
    } catch (err) {
      set({ error: (err as Error).message, loading: false });
    }
  },

  addSubscription: async (data) => {
    try {
      const created = await api<Subscription>("/subscriptions", {
        method: "POST",
        body: data,
      });
      set((s) => ({ items: [...s.items, created] }));
    } catch (err) {
      set({ error: (err as Error).message });
      throw err;
    }
  },

  updateSubscription: async (id, data) => {
    try {
      const updated = await api<Subscription>(`/subscriptions/${id}`, {
        method: "PATCH",
        body: data,
      });
      set((s) => ({
        items: s.items.map((item) => (item.id === id ? updated : item)),
      }));
    } catch (err) {
      set({ error: (err as Error).message });
      throw err;
    }
  },

  deleteSubscription: async (id) => {
    try {
      await api(`/subscriptions/${id}`, { method: "DELETE" });
      set((s) => ({ items: s.items.filter((item) => item.id !== id) }));
    } catch (err) {
      set({ error: (err as Error).message });
      throw err;
    }
  },

  setItems: (items) => set({ items }),
}));

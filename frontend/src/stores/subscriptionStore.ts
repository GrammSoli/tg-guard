import { create } from "zustand";
import type { Subscription } from "@/types/subscription";
import { api } from "@/lib/api";

interface SubscriptionStore {
  items: Subscription[];
  loading: boolean;
  error: string | null;
  filter: string;
  setFilter: (q: string) => void;
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
  setFilter: (q) => set({ filter: q }),

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

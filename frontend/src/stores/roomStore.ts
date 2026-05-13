import { create } from "zustand";
import type { BrandKey } from "@/types/subscription";
import type { RoomSummary } from "@/types/room";
import { api, ApiError } from "@/lib/api";

// Update a single room inside `rooms` from the latest activeDetail. Avoids
// the previous pattern of refetching the entire list on every mutation.
function syncRoomFromDetail(
  rooms: RoomSummary[],
  detail: RoomDetailData | null,
): RoomSummary[] {
  if (!detail) return rooms;
  const total = detail.services.reduce((acc, s) => acc + (s.amount || 0), 0);
  const perMember = detail.members.length > 0 ? total / detail.members.length : 0;
  let found = false;
  const next = rooms.map((r) => {
    if (r.id !== detail.id) return r;
    found = true;
    return {
      ...r,
      name: detail.name,
      members: detail.members.length,
      total_per_member: perMember,
      currency: detail.currency,
      services: detail.services.map((s) => ({ brand: s.brand })),
    } as RoomSummary;
  });
  return found ? next : rooms;
}

interface RoomDetailData {
  id: string;
  name: string;
  owner_id: number;
  invite_code: string;
  services: { id: number; brand: BrandKey; logo_url: string; name: string; amount: number; currency: string; note?: string; icon_name?: string; icon_color?: string; next_payment_at?: string }[];
  members: { user_id: number; name: string; username?: string; avatar?: string; has_paid: boolean; paid_at?: string }[];
  currency: string;
  total_per_member: number;
  total: number;
  is_owner: boolean;
  billing_day: number;
  created_at: string;
  last_reminded_at: string | null;
}

interface RoomStore {
  rooms: RoomSummary[];
  activeDetail: RoomDetailData | null;
  loading: boolean;
  error: string | null;
  fetchRooms: () => Promise<void>;
  fetchDetail: (id: string) => Promise<void>;
  create: (data: {
    name: string;
    currency: string;
    services: { brand: string; logo_url: string; name: string; amount: number; currency: string }[];
  }) => Promise<RoomSummary>;
  join: (inviteCode: string) => Promise<RoomSummary>;
  remind: (id: string) => Promise<{ reminded: number; members: string[] }>;
  markPaid: (roomId: string, userId: number) => Promise<void>;
  markUnpaid: (roomId: string, userId: number) => Promise<void>;
  deleteRoom: (id: string) => Promise<void>;
  addService: (roomId: string, svc: { brand: string; logo_url: string; name: string; amount: number; currency: string; note?: string; icon_name?: string; icon_color?: string }) => Promise<void>;
  removeService: (roomId: string, serviceId: number) => Promise<void>;
  removeMember: (roomId: string, userId: number) => Promise<void>;
  updateRoom: (roomId: string, data: { billing_day?: number }) => Promise<void>;
}

export const useRoomStore = create<RoomStore>((set, get) => ({
  rooms: [],
  activeDetail: null,
  loading: false,
  error: null,

  fetchRooms: async () => {
    set({ loading: true, error: null });
    try {
      const rooms = await api<RoomSummary[]>("/rooms");
      set({ rooms, loading: false });
    } catch (err) {
      set({ error: (err as Error).message, loading: false });
    }
  },

  fetchDetail: async (id: string) => {
    try {
      const detail = await api<RoomDetailData>(`/rooms/${id}`);
      set({ activeDetail: detail, error: null });
    } catch (err) {
      const e = err as ApiError;
      // 401 is handled globally by api.ts (reload); other errors deserve a
      // visible toast so the user understands why the sheet is empty.
      set({ activeDetail: null, error: e?.message ?? "load failed" });
      if (e?.status !== 401) {
        const { toast } = await import("sonner");
        if (e?.status === 403) toast.error("No access to this room");
        else toast.error("Failed to load room");
      }
    }
  },

  create: async (data) => {
    const summary = await api<RoomSummary>("/rooms", {
      method: "POST",
      body: data,
    });
    set((s) => ({ rooms: [...s.rooms, summary] }));
    return summary;
  },

  join: async (inviteCode: string) => {
    const summary = await api<RoomSummary>(`/rooms/join/${inviteCode}`, {
      method: "POST",
    });
    set((s) => ({
      rooms: s.rooms.some((r) => r.id === summary.id) ? s.rooms : [...s.rooms, summary],
    }));
    return summary;
  },

  remind: async (id: string) => {
    return api<{ reminded: number; members: string[] }>(`/rooms/${id}/remind`, {
      method: "POST",
    });
  },

  markPaid: async (roomId: string, userId: number) => {
    const detail = await api<RoomDetailData>(`/rooms/${roomId}/members/${userId}/pay`, {
      method: "PATCH",
    });
    set((s) => ({ activeDetail: detail, rooms: syncRoomFromDetail(s.rooms, detail) }));
  },

  markUnpaid: async (roomId: string, userId: number) => {
    const detail = await api<RoomDetailData>(`/rooms/${roomId}/members/${userId}/unpay`, {
      method: "PATCH",
    });
    set((s) => ({ activeDetail: detail, rooms: syncRoomFromDetail(s.rooms, detail) }));
  },

  deleteRoom: async (id: string) => {
    await api(`/rooms/${id}`, { method: "DELETE" });
    set((s) => ({
      rooms: s.rooms.filter((r) => r.id !== id),
      activeDetail: s.activeDetail?.id === id ? null : s.activeDetail,
    }));
  },

  addService: async (roomId, svc) => {
    await api(`/rooms/${roomId}/services`, { method: "POST", body: svc });
    await get().fetchDetail(roomId);
    set((s) => ({ rooms: syncRoomFromDetail(s.rooms, s.activeDetail) }));
  },

  removeService: async (roomId, serviceId) => {
    await api(`/rooms/${roomId}/services/${serviceId}`, { method: "DELETE" });
    await get().fetchDetail(roomId);
    set((s) => ({ rooms: syncRoomFromDetail(s.rooms, s.activeDetail) }));
  },

  removeMember: async (roomId, userId) => {
    await api(`/rooms/${roomId}/members/${userId}`, { method: "DELETE" });
    await get().fetchDetail(roomId);
    set((s) => ({ rooms: syncRoomFromDetail(s.rooms, s.activeDetail) }));
  },

  updateRoom: async (roomId, data) => {
    await api(`/rooms/${roomId}`, { method: "PATCH", body: data });
    await get().fetchDetail(roomId);
    set((s) => ({ rooms: syncRoomFromDetail(s.rooms, s.activeDetail) }));
  },
}));

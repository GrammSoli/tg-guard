import { create } from "zustand";

interface ModalState {
  // SharedRoomSheet
  activeRoomId: string | null;
  openRoom: (id: string) => void;
  closeRoom: () => void;

  // CreateRoomSheet
  createRoomOpen: boolean;
  openCreateRoom: () => void;
  closeCreateRoom: () => void;

  // AddSubscriptionSheet
  addSubOpen: boolean;
  addSubInitial: Record<string, unknown> | null;
  openAddSub: (initial?: Record<string, unknown> | null) => void;
  closeAddSub: () => void;

  // FilterSheet
  filterOpen: boolean;
  openFilter: () => void;
  closeFilter: () => void;
}

export const useModalStore = create<ModalState>((set) => ({
  // SharedRoomSheet
  activeRoomId: null,
  openRoom: (id) => set({ activeRoomId: id }),
  closeRoom: () => set({ activeRoomId: null }),

  // CreateRoomSheet
  createRoomOpen: false,
  openCreateRoom: () => set({ createRoomOpen: true }),
  closeCreateRoom: () => set({ createRoomOpen: false }),

  // AddSubscriptionSheet
  addSubOpen: false,
  addSubInitial: null,
  openAddSub: (initial = null) => set({ addSubOpen: true, addSubInitial: initial }),
  closeAddSub: () => set({ addSubOpen: false, addSubInitial: null }),

  // FilterSheet
  filterOpen: false,
  openFilter: () => set({ filterOpen: true }),
  closeFilter: () => set({ filterOpen: false }),
}));

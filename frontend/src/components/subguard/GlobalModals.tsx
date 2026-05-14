import { useEffect } from "react";
import { useModalStore } from "@/stores/modalStore";
import { useRoomStore } from "@/stores/roomStore";
import { useSubscriptionStore } from "@/stores/subscriptionStore";
import { SharedRoomSheet } from "./SharedRoomSheet";
import { CreateRoomSheet } from "./CreateRoomSheet";
import { AddSubscriptionSheet } from "./AddSubscriptionSheet";
import { FilterSheet } from "./FilterSheet";
import { hapticImpact, hapticNotification } from "@/lib/telegram";
import type { Subscription } from "@/types/subscription";

/**
 * Global modal/sheet container — mounted once in the root layout so that
 * sheets are always available regardless of which route is active.
 */
export function GlobalModals() {
  const activeRoomId = useModalStore((s) => s.activeRoomId);
  const closeRoom = useModalStore((s) => s.closeRoom);
  const createRoomOpen = useModalStore((s) => s.createRoomOpen);
  const closeCreateRoom = useModalStore((s) => s.closeCreateRoom);
  const addSubOpen = useModalStore((s) => s.addSubOpen);
  const addSubInitial = useModalStore((s) => s.addSubInitial);
  const closeAddSub = useModalStore((s) => s.closeAddSub);
  const filterOpen = useModalStore((s) => s.filterOpen);
  const closeFilter = useModalStore((s) => s.closeFilter);

  const fetchDetail = useRoomStore((s) => s.fetchDetail);
  const addSubscription = useSubscriptionStore((s) => s.addSubscription);
  const updateSubscription = useSubscriptionStore((s) => s.updateSubscription);
  const deleteSubscription = useSubscriptionStore((s) => s.deleteSubscription);

  // Auto-fetch room detail when a room is opened.
  // fetchDetail is a Zustand action — stable ref, listing it in deps
  // just risks HMR double-fires.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (activeRoomId) fetchDetail(activeRoomId);
  }, [activeRoomId]);

  const handleSave = async (data: Omit<Subscription, "id"> & { id?: string }) => {
    try {
      if (data.id) {
        await updateSubscription(data.id, data);
      } else {
        await addSubscription(data as Omit<Subscription, "id">);
      }
      hapticNotification("success");
    } catch {
      hapticNotification("error");
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await deleteSubscription(id);
      closeAddSub();
      hapticNotification("warning");
    } catch {
      hapticNotification("error");
    }
  };

  return (
    <>
      <SharedRoomSheet
        roomId={activeRoomId}
        open={!!activeRoomId}
        onOpenChange={(o) => {
          if (!o) {
            closeRoom();
            hapticImpact("light");
          }
        }}
      />

      <CreateRoomSheet
        open={createRoomOpen}
        onOpenChange={(o) => {
          if (!o) closeCreateRoom();
        }}
      />

      <AddSubscriptionSheet
        open={addSubOpen}
        onOpenChange={(o) => {
          if (!o) closeAddSub();
        }}
        initial={addSubInitial as Subscription | null}
        onSave={handleSave}
        onDelete={handleDelete}
      />

      <FilterSheet
        open={filterOpen}
        onOpenChange={(o) => {
          if (!o) closeFilter();
        }}
      />
    </>
  );
}

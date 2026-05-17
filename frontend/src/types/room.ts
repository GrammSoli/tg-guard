import type { BrandKey } from "@/types/subscription";

export interface RoomService {
  brand: BrandKey;
  name: string;
  amount: number;
  currency: string;
  note?: string;
  icon_name?: string;
  icon_color?: string;
  nextPaymentAt?: string;
}

export interface RoomMember {
  uid: string;
  name: string;
  avatar?: string;
  hasPaid: boolean;
  paidAt?: string;
}

export interface Room {
  id: string;
  name: string;
  ownerId: string;
  inviteCode: string;
  services: RoomService[];
  members: RoomMember[];
  currency: string;
  createdAt: string;
}

export interface RoomSummary {
  id: string;
  name: string;
  members: number;
  total_per_member: number;
  currency: string;
  // icon_name/icon_color carry the user's IconPicker choice for
  // custom services (brand="default"). The dashboard room card uses
  // them to render the chosen icon instead of falling back to the
  // first-letter placeholder of "default" → "D".
  services: {
    brand: BrandKey;
    icon_name?: string;
    icon_color?: string;
  }[];
}

export function roomToSummary(room: Room): RoomSummary {
  const total = room.services.reduce((s, svc) => s + svc.amount, 0);
  const perMember = room.members.length > 0 ? total / room.members.length : 0;
  return {
    id: room.id,
    name: room.name,
    members: room.members.length,
    total_per_member: Math.round(perMember * 100) / 100,
    currency: room.currency,
    services: room.services.map((s) => ({
      brand: s.brand,
      icon_name: s.icon_name,
      icon_color: s.icon_color,
    })),
  };
}

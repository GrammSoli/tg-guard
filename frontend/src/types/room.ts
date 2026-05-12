import type { BrandKey } from "@/types/subscription";

export interface RoomService {
  brand: BrandKey;
  name: string;
  amount: number;
  currency: string;
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
  services: { brand: BrandKey }[];
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
    services: room.services.map((s) => ({ brand: s.brand })),
  };
}

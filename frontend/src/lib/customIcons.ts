/**
 * Allow-listed catalogue of icons and background colours for custom
 * subscriptions. Keeping it explicit (vs. allowing arbitrary strings from
 * the API) means:
 *   - tree-shaking still works — only used lucide icons land in the bundle
 *   - unknown / malicious names from old data or bad clients can never
 *     render unintended icons or arbitrary class names
 *
 * To add a new icon: import it from lucide-react and append to ICON_LIST.
 * To add a new colour: append to COLOR_LIST. Keep ids short and lowercase.
 */

import {
  Bookmark,
  Car,
  Coffee,
  CreditCard,
  Dumbbell,
  Flame,
  Gamepad2,
  Gift,
  GraduationCap,
  Heart,
  Home,
  Lightbulb,
  Music,
  PawPrint,
  Plane,
  ShoppingBag,
  Sparkles,
  Tv,
  Utensils,
  Zap,
  type LucideIcon,
} from "lucide-react";

export interface CustomIconOption {
  name: string;
  Icon: LucideIcon;
}

export const ICON_LIST: CustomIconOption[] = [
  { name: "Home", Icon: Home },
  { name: "Car", Icon: Car },
  { name: "Heart", Icon: Heart },
  { name: "Dumbbell", Icon: Dumbbell },
  { name: "Gamepad2", Icon: Gamepad2 },
  { name: "GraduationCap", Icon: GraduationCap },
  { name: "Plane", Icon: Plane },
  { name: "Tv", Icon: Tv },
  { name: "Music", Icon: Music },
  { name: "Coffee", Icon: Coffee },
  { name: "Zap", Icon: Zap },
  { name: "Flame", Icon: Flame },
  { name: "Sparkles", Icon: Sparkles },
  { name: "ShoppingBag", Icon: ShoppingBag },
  { name: "Utensils", Icon: Utensils },
  { name: "PawPrint", Icon: PawPrint },
  { name: "Gift", Icon: Gift },
  { name: "Bookmark", Icon: Bookmark },
  { name: "CreditCard", Icon: CreditCard },
  { name: "Lightbulb", Icon: Lightbulb },
];

/**
 * Map from the stored string name to the actual lucide component. O(1)
 * lookup at render time; unknown names return undefined so the caller
 * can fall back gracefully to the letter avatar.
 */
export const ICON_MAP: Record<string, LucideIcon> = Object.fromEntries(
  ICON_LIST.map((opt) => [opt.name, opt.Icon]),
);

export interface CustomColorOption {
  /** Stored id ("blue", "red"…) — what the backend persists. */
  id: string;
  /** Tailwind background class for the avatar circle. */
  bg: string;
  /** Slightly darker variant for the picker's selected outline / shadow. */
  ring: string;
}

export const COLOR_LIST: CustomColorOption[] = [
  { id: "blue", bg: "bg-blue-500", ring: "ring-blue-300" },
  { id: "red", bg: "bg-red-500", ring: "ring-red-300" },
  { id: "emerald", bg: "bg-emerald-500", ring: "ring-emerald-300" },
  { id: "purple", bg: "bg-purple-500", ring: "ring-purple-300" },
  { id: "orange", bg: "bg-orange-500", ring: "ring-orange-300" },
  { id: "pink", bg: "bg-pink-500", ring: "ring-pink-300" },
];

export const COLOR_MAP: Record<string, CustomColorOption> = Object.fromEntries(
  COLOR_LIST.map((c) => [c.id, c]),
);

export const DEFAULT_ICON_NAME = "Sparkles";
export const DEFAULT_ICON_COLOR = "purple";

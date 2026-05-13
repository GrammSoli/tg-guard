import { useTranslation } from "react-i18next";
import { Check } from "lucide-react";
import { COLOR_LIST, ICON_LIST } from "@/lib/customIcons";
import { hapticSelection } from "@/lib/telegram";

interface Props {
  iconName: string;
  iconColor: string;
  onChange: (next: { iconName: string; iconColor: string }) => void;
}

/**
 * IconPicker — two compact grids for choosing the avatar appearance of a
 * custom (Brand === "default") subscription. Icons render in a 5-column
 * grid that wraps to as many rows as needed; colours wrap into a single
 * row (or two, on very narrow popovers).
 *
 * Why a grid instead of a horizontal scroll strip? Inside a desktop
 * Popover the previous horizontal-scroll layout was unreachable with a
 * mouse wheel — wheel events scroll vertically, not horizontally. A
 * wrapping grid + bounded max-height with vertical overflow gives the
 * intuitive scrollbar-with-wheel UX on desktop and stays touch-friendly
 * on mobile.
 *
 * The picker doesn't own state — the parent form holds iconName/iconColor
 * and re-feeds them in. That keeps the picker pure and the avatar preview
 * in the form header trivially live.
 */
export function IconPicker({ iconName, iconColor, onChange }: Props) {
  const { t } = useTranslation();

  return (
    <div className="space-y-3">
      <p className="px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
        {t("modal.appearance")}
      </p>

      {/* Icons — 5-col grid, vertical overflow when the list grows */}
      <div className="grid max-h-48 grid-cols-5 gap-2 overflow-y-auto">
        {ICON_LIST.map(({ name, Icon }) => {
          const active = name === iconName;
          return (
            <button
              key={name}
              type="button"
              onClick={() => {
                hapticSelection();
                onChange({ iconName: name, iconColor });
              }}
              aria-label={name}
              aria-pressed={active}
              className={`flex h-10 w-full items-center justify-center rounded-xl transition-colors active:scale-95 ${
                active
                  ? "bg-foreground text-background"
                  : "bg-surface text-muted-foreground hover:text-foreground"
              }`}
            >
              <Icon size={18} strokeWidth={2.25} />
            </button>
          );
        })}
      </div>

      {/* Colours — flex-wrap so a narrow Popover doesn't clip them */}
      <div className="flex flex-wrap gap-2">
        {COLOR_LIST.map(({ id, bg, ring }) => {
          const active = id === iconColor;
          return (
            <button
              key={id}
              type="button"
              onClick={() => {
                hapticSelection();
                onChange({ iconName, iconColor: id });
              }}
              aria-label={id}
              aria-pressed={active}
              className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full transition-transform active:scale-90 ${bg} ${
                active ? `ring-2 ring-offset-2 ring-offset-background ${ring}` : ""
              }`}
            >
              {active && <Check className="h-4 w-4 text-white" />}
            </button>
          );
        })}
      </div>
    </div>
  );
}

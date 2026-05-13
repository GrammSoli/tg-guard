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
 * IconPicker — two compact strips for choosing the avatar appearance of a
 * custom (Brand === "default") subscription. Icons scroll horizontally on
 * narrow screens; colours stay in a single row.
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

      {/* Icons — horizontal scroll, 5 visible at a time */}
      <div className="-mx-1 overflow-x-auto px-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
        <div className="flex gap-2">
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
                className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl transition-colors active:scale-95 ${
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
      </div>

      {/* Colours — single row of dots */}
      <div className="flex gap-2">
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

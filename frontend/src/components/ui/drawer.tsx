import * as React from "react";
import { Drawer as DrawerPrimitive } from "vaul";

import { cn } from "@/lib/utils";
import { useTelegramViewportHeight } from "@/hooks/use-telegram-viewport";

const Drawer = ({
  shouldScaleBackground = true,
  repositionInputs = false,
  ...props
}: React.ComponentProps<typeof DrawerPrimitive.Root>) => (
  // repositionInputs={false}: vaul's built-in iOS keyboard handler
  // translates the drawer upwards when an input is focused. With
  // `interactive-widget=resizes-content` already set on the viewport
  // meta, the browser shrinks the layout viewport and `position: fixed;
  // bottom: 0` already anchors above the keyboard. Letting vaul shift
  // the drawer on TOP of that double-shifts it and leaves a black void
  // between the drawer and the keyboard. Disable vaul's handling and
  // trust the platform.
  <DrawerPrimitive.Root
    shouldScaleBackground={shouldScaleBackground}
    repositionInputs={repositionInputs}
    {...props}
  />
);
Drawer.displayName = "Drawer";

const DrawerTrigger = DrawerPrimitive.Trigger;

const DrawerPortal = DrawerPrimitive.Portal;

const DrawerClose = DrawerPrimitive.Close;

const DrawerOverlay = React.forwardRef<
  React.ElementRef<typeof DrawerPrimitive.Overlay>,
  React.ComponentPropsWithoutRef<typeof DrawerPrimitive.Overlay>
>(({ className, ...props }, ref) => (
  <DrawerPrimitive.Overlay
    ref={ref}
    className={cn("fixed inset-0 z-50 bg-black/80", className)}
    {...props}
  />
));
DrawerOverlay.displayName = DrawerPrimitive.Overlay.displayName;

const DrawerContent = React.forwardRef<
  React.ElementRef<typeof DrawerPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof DrawerPrimitive.Content>
>(({ className, children, style, ...props }, ref) => {
  // On-screen keyboard handling:
  //   - `interactive-widget=resizes-content` in index.html makes modern
  //     browsers shrink the layout viewport with the keyboard, so a
  //     `position: fixed; bottom: 0` anchors above the keyboard out of
  //     the box. That fix alone covers iOS 16.4+ / Chrome WebView 108+.
  //   - For older platforms `window.innerHeight` stays at the original
  //     layout-viewport height while Telegram still reports the visible
  //     height via viewportStableHeight (tgVh). The difference IS the
  //     keyboard. We shift the drawer up by that amount and clamp its
  //     max-height so it sits entirely above the keyboard.
  const tgVh = useTelegramViewportHeight();
  const layoutVh = typeof window !== "undefined" ? window.innerHeight : 0;
  const keyboardOffset =
    tgVh > 0 && layoutVh > 0 ? Math.max(0, layoutVh - tgVh) : 0;
  const mergedStyle = {
    ...(tgVh > 0 ? { maxHeight: `${Math.round(tgVh * 0.85)}px` } : null),
    ...(keyboardOffset > 0 ? { bottom: `${keyboardOffset}px` } : null),
    ...style,
  };
  return (
    <DrawerPortal>
      <DrawerOverlay />
      <DrawerPrimitive.Content
        ref={ref}
        className={cn(
          "fixed inset-x-0 bottom-0 z-50 mt-24 flex max-h-[85dvh] flex-col rounded-t-[10px] border bg-background",
          className,
        )}
        style={mergedStyle}
        {...props}
      >
        <div className="mx-auto mt-4 h-2 w-[100px] shrink-0 rounded-full bg-muted" />
        <div className="flex-1 overflow-y-auto">
          {children}
        </div>
      </DrawerPrimitive.Content>
    </DrawerPortal>
  );
});
DrawerContent.displayName = "DrawerContent";

const DrawerHeader = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
  <div className={cn("grid gap-1.5 p-4 text-center sm:text-left", className)} {...props} />
);
DrawerHeader.displayName = "DrawerHeader";

const DrawerFooter = ({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
  <div className={cn("mt-auto flex shrink-0 flex-col gap-2 p-4 pb-8", className)} {...props} />
);
DrawerFooter.displayName = "DrawerFooter";

const DrawerTitle = React.forwardRef<
  React.ElementRef<typeof DrawerPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof DrawerPrimitive.Title>
>(({ className, ...props }, ref) => (
  <DrawerPrimitive.Title
    ref={ref}
    className={cn("text-lg font-semibold leading-none tracking-tight", className)}
    {...props}
  />
));
DrawerTitle.displayName = DrawerPrimitive.Title.displayName;

const DrawerDescription = React.forwardRef<
  React.ElementRef<typeof DrawerPrimitive.Description>,
  React.ComponentPropsWithoutRef<typeof DrawerPrimitive.Description>
>(({ className, ...props }, ref) => (
  <DrawerPrimitive.Description
    ref={ref}
    className={cn("text-sm text-muted-foreground", className)}
    {...props}
  />
));
DrawerDescription.displayName = DrawerPrimitive.Description.displayName;

export {
  Drawer,
  DrawerPortal,
  DrawerOverlay,
  DrawerTrigger,
  DrawerClose,
  DrawerContent,
  DrawerHeader,
  DrawerFooter,
  DrawerTitle,
  DrawerDescription,
};

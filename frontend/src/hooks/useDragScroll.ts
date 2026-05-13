import { useRef, useCallback, useEffect } from "react";

/**
 * useDragScroll — enables horizontal drag-to-scroll on desktop.
 *
 * Returns a ref to attach to the scrollable container.
 * Automatically prevents click events when the user drags more than
 * `CLICK_THRESHOLD` pixels, so filter buttons don't fire on swipe.
 */
const CLICK_THRESHOLD = 5;

export function useDragScroll<T extends HTMLElement = HTMLDivElement>() {
  const ref = useRef<T>(null);
  const state = useRef({
    isDown: false,
    startX: 0,
    scrollLeft: 0,
    dragged: false,
  });

  const onMouseDown = useCallback((e: MouseEvent) => {
    const el = ref.current;
    if (!el) return;
    state.current.isDown = true;
    state.current.dragged = false;
    state.current.startX = e.pageX - el.offsetLeft;
    state.current.scrollLeft = el.scrollLeft;
    el.style.cursor = "grabbing";
  }, []);

  const onMouseMove = useCallback((e: MouseEvent) => {
    const el = ref.current;
    if (!el || !state.current.isDown) return;
    e.preventDefault();
    const x = e.pageX - el.offsetLeft;
    const walk = x - state.current.startX;
    if (Math.abs(walk) > CLICK_THRESHOLD) {
      state.current.dragged = true;
    }
    el.scrollLeft = state.current.scrollLeft - walk;
  }, []);

  const onMouseUp = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    state.current.isDown = false;
    el.style.cursor = "";
  }, []);

  const onMouseLeave = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    state.current.isDown = false;
    el.style.cursor = "";
  }, []);

  // Capture click events and cancel them if we dragged
  const onClick = useCallback((e: MouseEvent) => {
    if (state.current.dragged) {
      e.preventDefault();
      e.stopPropagation();
      state.current.dragged = false;
    }
  }, []);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;

    el.addEventListener("mousedown", onMouseDown);
    el.addEventListener("mousemove", onMouseMove);
    el.addEventListener("mouseup", onMouseUp);
    el.addEventListener("mouseleave", onMouseLeave);
    // Use capture phase so we intercept before button onClick fires
    el.addEventListener("click", onClick, true);

    return () => {
      el.removeEventListener("mousedown", onMouseDown);
      el.removeEventListener("mousemove", onMouseMove);
      el.removeEventListener("mouseup", onMouseUp);
      el.removeEventListener("mouseleave", onMouseLeave);
      el.removeEventListener("click", onClick, true);
    };
  }, [onMouseDown, onMouseMove, onMouseUp, onMouseLeave, onClick]);

  return ref;
}

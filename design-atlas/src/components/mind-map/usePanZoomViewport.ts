import { useCallback, useEffect, useRef, useState } from "react";
import type { PointerEvent as ReactPointerEvent } from "react";

const MIN_SCALE = 0.55;
const MAX_SCALE = 1.8;
const WHEEL_ZOOM_SENSITIVITY = 0.0014;
const DEFAULT_TRANSFORM: ViewTransform = { x: 0, y: 0, scale: 1 };

type ViewTransform = {
  x: number;
  y: number;
  scale: number;
};

type DragState = {
  pointerId: number;
  startX: number;
  startY: number;
  originX: number;
  originY: number;
};

export function usePanZoomViewport() {
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const detachWheelListenerRef = useRef<(() => void) | null>(null);
  const dragStateRef = useRef<DragState | null>(null);
  const transformRef = useRef<ViewTransform>(DEFAULT_TRANSFORM);
  const [transform, setTransformState] = useState<ViewTransform>(DEFAULT_TRANSFORM);
  const [isPanning, setIsPanning] = useState(false);

  useEffect(() => {
    return () => {
      detachWheelListenerRef.current?.();
    };
  }, []);

  const setViewportElement = useCallback((viewport: HTMLDivElement | null) => {
    detachWheelListenerRef.current?.();
    detachWheelListenerRef.current = null;
    viewportRef.current = viewport;

    if (!viewport) {
      return;
    }

    const mountedViewport = viewport;

    function handleNativeWheel(event: WheelEvent) {
      event.preventDefault();
      event.stopPropagation();

      const viewportRect = mountedViewport.getBoundingClientRect();
      const pointX = event.clientX - viewportRect.left;
      const pointY = event.clientY - viewportRect.top;
      const currentTransform = transformRef.current;
      const nextScale = currentTransform.scale * Math.exp(-event.deltaY * WHEEL_ZOOM_SENSITIVITY);

      zoomToPoint(nextScale, pointX, pointY);
    }

    mountedViewport.addEventListener("wheel", handleNativeWheel, { passive: false });
    detachWheelListenerRef.current = () => mountedViewport.removeEventListener("wheel", handleNativeWheel);
  }, []);

  function setTransform(nextTransform: ViewTransform) {
    const normalizedTransform = {
      ...nextTransform,
      scale: clamp(nextTransform.scale, MIN_SCALE, MAX_SCALE),
    };

    transformRef.current = normalizedTransform;
    setTransformState(normalizedTransform);
  }

  function handlePointerDown(event: ReactPointerEvent<HTMLDivElement>) {
    if (event.button !== 0 || isInteractiveTarget(event.target)) {
      return;
    }

    const currentTransform = transformRef.current;
    dragStateRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      originX: currentTransform.x,
      originY: currentTransform.y,
    };

    event.currentTarget.setPointerCapture(event.pointerId);
    setIsPanning(true);
  }

  function handlePointerMove(event: ReactPointerEvent<HTMLDivElement>) {
    const dragState = dragStateRef.current;

    if (!dragState || dragState.pointerId !== event.pointerId) {
      return;
    }

    setTransform({
      ...transformRef.current,
      x: dragState.originX + event.clientX - dragState.startX,
      y: dragState.originY + event.clientY - dragState.startY,
    });
  }

  function handlePointerUp(event: ReactPointerEvent<HTMLDivElement>) {
    finishPan(event);
  }

  function handlePointerCancel(event: ReactPointerEvent<HTMLDivElement>) {
    finishPan(event);
  }

  function zoomToPoint(nextScale: number, pointX: number, pointY: number) {
    const currentTransform = transformRef.current;
    const normalizedScale = clamp(nextScale, MIN_SCALE, MAX_SCALE);
    const scaleRatio = normalizedScale / currentTransform.scale;

    setTransform({
      scale: normalizedScale,
      x: pointX - (pointX - currentTransform.x) * scaleRatio,
      y: pointY - (pointY - currentTransform.y) * scaleRatio,
    });
  }

  function finishPan(event: ReactPointerEvent<HTMLDivElement>) {
    const dragState = dragStateRef.current;

    if (!dragState || dragState.pointerId !== event.pointerId) {
      return;
    }

    dragStateRef.current = null;

    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }

    setIsPanning(false);
  }

  return {
    isPanning,
    transform,
    setViewportElement,
    handlePointerCancel,
    handlePointerDown,
    handlePointerMove,
    handlePointerUp,
  };
}

function isInteractiveTarget(target: EventTarget) {
  return target instanceof Element && Boolean(target.closest("button, a, input, textarea, select"));
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

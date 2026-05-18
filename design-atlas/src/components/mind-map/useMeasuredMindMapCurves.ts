import { useEffect, useRef, useState } from "react";

type MindMapCurve = {
  id: string;
  accent: string;
  path: string;
};

type CurvePoint = {
  x: number;
  y: number;
};

const SECTION_COLORS = ["rgba(124, 58, 237, 0.58)", "rgba(8, 145, 178, 0.52)", "rgba(5, 150, 105, 0.52)"];

export function useMeasuredMindMapCurves() {
  const canvasRef = useRef<HTMLDivElement | null>(null);
  const recalculateFrameRef = useRef<number | null>(null);
  const resizeObserverRef = useRef<ResizeObserver | null>(null);
  const rootRef = useRef<HTMLDivElement | null>(null);
  const sectionRefs = useRef(new Map<string, HTMLElement>());
  const leafRefs = useRef(new Map<string, HTMLElement>());
  const [curves, setCurves] = useState<MindMapCurve[]>([]);

  useEffect(() => {
    const canvas = canvasRef.current;
    const root = rootRef.current;

    if (!canvas || !root) {
      throw new Error("Mind map curve anchors are not mounted.");
    }

    const resizeObserver = new ResizeObserver(scheduleRecalculateCurves);
    const observedElements = [canvas, root, ...sectionRefs.current.values(), ...leafRefs.current.values()];

    resizeObserverRef.current = resizeObserver;

    for (const element of observedElements) {
      resizeObserver.observe(element);
    }

    scheduleRecalculateCurves();
    window.addEventListener("resize", scheduleRecalculateCurves);

    return () => {
      if (recalculateFrameRef.current !== null) {
        cancelAnimationFrame(recalculateFrameRef.current);
      }

      resizeObserver.disconnect();
      resizeObserverRef.current = null;
      window.removeEventListener("resize", scheduleRecalculateCurves);
    };
  }, []);

  function registerSection(id: string) {
    return registerElement(sectionRefs.current, id, scheduleRecalculateCurves, resizeObserverRef);
  }

  function registerLeaf(id: string) {
    return registerElement(leafRefs.current, id, scheduleRecalculateCurves, resizeObserverRef);
  }

  function scheduleRecalculateCurves() {
    if (recalculateFrameRef.current !== null) {
      cancelAnimationFrame(recalculateFrameRef.current);
    }

    recalculateFrameRef.current = requestAnimationFrame(() => {
      recalculateFrameRef.current = null;
      recalculateCurves();
    });
  }

  function recalculateCurves() {
    const canvas = canvasRef.current;
    const root = rootRef.current;

    if (!canvas || !root) {
      throw new Error("Mind map curve anchors are not mounted.");
    }

    const rootPoint = getRightCenter(root, canvas);
    const nextCurves: MindMapCurve[] = [];

    Array.from(sectionRefs.current.entries()).forEach(([sectionId, section], sectionIndex) => {
      const sectionPoint = getLeftCenter(section, canvas);
      const accent = SECTION_COLORS[sectionIndex % SECTION_COLORS.length];

      nextCurves.push({
        id: `root-${sectionId}`,
        accent,
        path: createBezierPath(rootPoint, sectionPoint),
      });

      for (const [leafId, leaf] of leafRefs.current.entries()) {
        if (!leafId.startsWith(`${sectionId}:`)) {
          continue;
        }

        nextCurves.push({
          id: `section-${leafId}`,
          accent,
          path: createBezierPath(getRightCenter(section, canvas), getLeftCenter(leaf, canvas)),
        });
      }
    });

    setCurves(nextCurves);
  }

  return {
    canvasRef,
    curves,
    registerLeaf,
    registerSection,
    rootRef,
  };
}

function registerElement<T extends HTMLElement>(
  registry: Map<string, T>,
  id: string,
  onRegisterChange: () => void,
  resizeObserverRef: React.MutableRefObject<ResizeObserver | null>,
) {
  return (element: T | null) => {
    const previousElement = registry.get(id);

    if (previousElement && previousElement !== element) {
      resizeObserverRef.current?.unobserve(previousElement);
    }

    if (element) {
      registry.set(id, element);
      resizeObserverRef.current?.observe(element);
      onRegisterChange();
      return;
    }

    registry.delete(id);
    onRegisterChange();
  };
}

function createBezierPath(start: CurvePoint, end: CurvePoint) {
  const distance = Math.max(80, end.x - start.x);
  const controlOffset = Math.max(80, distance * 0.42);

  return `M ${start.x} ${start.y} C ${start.x + controlOffset} ${start.y}, ${end.x - controlOffset} ${end.y}, ${end.x} ${end.y}`;
}

function getLeftCenter(element: HTMLElement, canvas: HTMLElement): CurvePoint {
  const elementRect = element.getBoundingClientRect();
  const canvasRect = canvas.getBoundingClientRect();
  const scale = getCanvasScale(canvas);

  return {
    x: (elementRect.left - canvasRect.left) / scale,
    y: (elementRect.top - canvasRect.top + elementRect.height / 2) / scale,
  };
}

function getRightCenter(element: HTMLElement, canvas: HTMLElement): CurvePoint {
  const elementRect = element.getBoundingClientRect();
  const canvasRect = canvas.getBoundingClientRect();
  const scale = getCanvasScale(canvas);

  return {
    x: (elementRect.right - canvasRect.left) / scale,
    y: (elementRect.top - canvasRect.top + elementRect.height / 2) / scale,
  };
}

function getCanvasScale(canvas: HTMLElement) {
  const rect = canvas.getBoundingClientRect();

  if (canvas.offsetWidth === 0) {
    throw new Error("Mind map canvas has zero width.");
  }

  return rect.width / canvas.offsetWidth;
}

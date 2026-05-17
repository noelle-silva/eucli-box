import type { CSSProperties, HTMLAttributes, PointerEvent, ReactNode } from "react";

type SpotlightPanelProps = HTMLAttributes<HTMLElement> & {
  children: ReactNode;
  className?: string;
  accent?: string;
  tag?: "div" | "article" | "section" | "aside" | "header";
  style?: CSSProperties;
};

export function SpotlightPanel({
  children,
  className = "",
  accent = "rgba(124, 58, 237, 0.28)",
  tag = "div",
  style,
  onPointerMove,
  ...props
}: SpotlightPanelProps) {
  const Element = tag;

  function handlePointerMove(event: PointerEvent<HTMLElement>) {
    const rect = event.currentTarget.getBoundingClientRect();
    event.currentTarget.style.setProperty("--spotlight-x", `${event.clientX - rect.left}px`);
    event.currentTarget.style.setProperty("--spotlight-y", `${event.clientY - rect.top}px`);
    onPointerMove?.(event);
  }

  return (
    <Element
      className={`spotlight-panel ${className}`}
      onPointerMove={handlePointerMove}
      style={{ "--spotlight-accent": accent, ...style } as CSSProperties}
      {...props}
    >
      {children}
    </Element>
  );
}

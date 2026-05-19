import type { CSSProperties, HTMLAttributes, PointerEvent, ReactNode } from "react";

type SpotlightCardProps = HTMLAttributes<HTMLElement> & {
  children: ReactNode;
  className?: string;
  spotlightColor?: string;
  tag?: "article" | "button" | "section" | "div";
  type?: "button" | "submit" | "reset";
};

export function SpotlightCard({ children, className = "", spotlightColor = "rgba(255,255,255,0.55)", tag = "article", onPointerMove, style, ...props }: SpotlightCardProps) {
  const Element = tag;

  function handlePointerMove(event: PointerEvent<HTMLElement>) {
    const rect = event.currentTarget.getBoundingClientRect();
    event.currentTarget.style.setProperty("--mouse-x", `${event.clientX - rect.left}px`);
    event.currentTarget.style.setProperty("--mouse-y", `${event.clientY - rect.top}px`);
    event.currentTarget.style.setProperty("--spotlight-color", spotlightColor);
    onPointerMove?.(event);
  }

  return (
    <Element className={`spotlight-card ${className}`} onPointerMove={handlePointerMove} style={style as CSSProperties} {...props}>
      {children}
    </Element>
  );
}

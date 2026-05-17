import type { ReactNode } from "react";

type GradientTextProps = {
  children: ReactNode;
  className?: string;
};

export function GradientText({ children, className = "" }: GradientTextProps) {
  return <span className={`gradient-text ${className}`}>{children}</span>;
}

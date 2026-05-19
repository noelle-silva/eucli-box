type ShinyTextProps = {
  children: string;
  className?: string;
};

export function ShinyText({ children, className = "" }: ShinyTextProps) {
  return <span className={`shiny-text ${className}`}>{children}</span>;
}

type ShinyTextProps = {
  text: string;
  className?: string;
};

export function ShinyText({ text, className = "" }: ShinyTextProps) {
  return <span className={`shiny-text ${className}`}>{text}</span>;
}

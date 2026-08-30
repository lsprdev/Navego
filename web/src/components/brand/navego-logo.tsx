import Link from "next/link";

import { Navy } from "@/components/brand/navy";
import { cn } from "@/lib/utils";

type NavegoLogoProps = {
  className?: string;
  compact?: boolean;
  href?: string;
};

export function NavegoLogo({
  className,
  compact = false,
  href = "/",
}: NavegoLogoProps) {
  return (
    <Link
      href={href}
      className={cn("group/logo flex w-fit items-center gap-2.5", className)}
      aria-label="Navego — página inicial"
    >
      <span className="brand-mark flex size-9 items-center justify-center rounded-xl border bg-card">
        <Navy
          className="size-7 transition-transform duration-300 group-hover/logo:-rotate-3 group-hover/logo:scale-105"
          interactive={false}
          decorative
        />
      </span>
      {compact ? null : (
        <span className="flex flex-col leading-none">
          <span className="text-[17px] font-semibold tracking-[-0.04em]">Navego</span>
          <span className="mt-1 hidden font-mono text-[8px] uppercase tracking-[0.23em] text-muted-foreground sm:block">
            Browser control
          </span>
        </span>
      )}
    </Link>
  );
}

"use client";

import { useEffect, useId, useRef } from "react";

import { cn } from "@/lib/utils";

type NavyProps = {
  className?: string;
  interactive?: boolean;
  decorative?: boolean;
};

export function Navy({
  className,
  interactive = true,
  decorative = false,
}: NavyProps) {
  const rootRef = useRef<HTMLSpanElement>(null);
  const clipID = `navy-${useId().replaceAll(":", "")}`;

  useEffect(() => {
    const root = rootRef.current;
    if (!root || !interactive) return;

    const reducedMotion = window.matchMedia(
      "(prefers-reduced-motion: reduce)",
    );
    let frame = 0;

    function reset() {
      if (!root) return;
      root.style.setProperty("--navy-eye-x", "0px");
      root.style.setProperty("--navy-eye-y", "0px");
      root.style.setProperty("--navy-tilt", "0deg");
    }

    function followPointer(event: PointerEvent) {
      if (!root || reducedMotion.matches) return;
      window.cancelAnimationFrame(frame);
      frame = window.requestAnimationFrame(() => {
        const bounds = root.getBoundingClientRect();
        const centerX = bounds.left + bounds.width / 2;
        const centerY = bounds.top + bounds.height / 2;
        const x = Math.max(-1, Math.min(1, (event.clientX - centerX) / 260));
        const y = Math.max(-1, Math.min(1, (event.clientY - centerY) / 220));
        root.style.setProperty("--navy-eye-x", `${x * 8}px`);
        root.style.setProperty("--navy-eye-y", `${y * 7}px`);
        root.style.setProperty("--navy-tilt", `${x * 2.2}deg`);
      });
    }

    window.addEventListener("pointermove", followPointer, { passive: true });
    window.addEventListener("blur", reset);
    reducedMotion.addEventListener("change", reset);
    return () => {
      window.cancelAnimationFrame(frame);
      window.removeEventListener("pointermove", followPointer);
      window.removeEventListener("blur", reset);
      reducedMotion.removeEventListener("change", reset);
    };
  }, [interactive]);

  return (
    <span
      ref={rootRef}
      className={cn("navy-root inline-grid", className)}
      aria-hidden={decorative ? true : undefined}
      role={decorative ? undefined : "img"}
      aria-label={decorative ? undefined : "Navy, mascote do Navego"}
    >
      <svg
        viewBox="0 0 240 180"
        xmlns="http://www.w3.org/2000/svg"
        className="size-full overflow-visible"
      >
        <defs>
          <clipPath id={clipID}>
            <path d="M19 145 43 75C49 55 64 43 84 43h74c20 0 35 12 42 32l23 70c4 13-5 26-19 26H39c-14 0-24-13-20-26Z" />
          </clipPath>
          <linearGradient id={`${clipID}-shine`} x1="45" y1="42" x2="193" y2="174">
            <stop offset="0" stopColor="white" />
            <stop offset="1" stopColor="#d7dbd9" />
          </linearGradient>
        </defs>

        <g className="navy-body">
          <path
            d="M19 145 43 75C49 55 64 43 84 43h74c20 0 35 12 42 32l23 70c4 13-5 26-19 26H39c-14 0-24-13-20-26Z"
            fill={`url(#${clipID}-shine)`}
          />
          <path
            d="M48 75c6-17 19-25 37-25h72c17 0 30 9 36 25"
            fill="none"
            stroke="white"
            strokeOpacity=".68"
            strokeWidth="3"
            strokeLinecap="round"
          />
          <path
            d="M35 151h171"
            fill="none"
            stroke="var(--primary)"
            strokeWidth="5"
            strokeLinecap="round"
            opacity=".95"
          />
        </g>

        <g clipPath={`url(#${clipID})`} className="navy-eyes">
          <g className="navy-eye">
            <rect
              x="91"
              y="79"
              width="21"
              height="44"
              rx="10.5"
              fill="#090909"
              transform="rotate(-13 101.5 101)"
            />
          </g>
          <g className="navy-eye navy-eye-secondary">
            <rect
              x="139"
              y="73"
              width="20"
              height="42"
              rx="10"
              fill="#090909"
              transform="rotate(-13 149 94)"
            />
          </g>
        </g>
      </svg>
    </span>
  );
}

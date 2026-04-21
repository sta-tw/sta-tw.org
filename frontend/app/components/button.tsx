import { Slot } from "radix-ui";
import type { ComponentPropsWithoutRef } from "react";
import { twMerge } from "tailwind-merge";

interface ButtonProps extends ComponentPropsWithoutRef<"button"> {
  asChild?: boolean;
}

export default function Button({ asChild, className, children, ...props }: ButtonProps) {
  const Comp = asChild ? Slot.Root : "button";
  return (
    <Comp
      className={twMerge(
        "inline-flex h-12 items-center justify-center rounded-xl bg-button px-6 font-serif text-xl text-button-foreground transition-colors hover:bg-button-hover active:bg-button-active disabled:pointer-events-none disabled:opacity-50",
        className
      )}
      {...props}
    >
      {children}
    </Comp>
  );
}

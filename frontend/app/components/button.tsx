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
        "inline-flex h-12 items-center justify-center rounded-xl bg-[#363535] px-6 font-serif text-xl text-white transition-colors hover:bg-[#4a4949] active:bg-[#252424] disabled:opacity-50 disabled:pointer-events-none",
        className
      )}
      {...props}
    >
      {children}
    </Comp>
  );
}

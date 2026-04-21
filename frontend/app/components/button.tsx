import { Slot } from "radix-ui";
import type { ComponentPropsWithoutRef } from "react";

interface ButtonProps extends ComponentPropsWithoutRef<"button"> {
  asChild?: boolean;
}

export default function Button({ asChild, className = "", children, ...props }: ButtonProps) {
  const Comp = asChild ? Slot.Root : "button";
  return (
    <Comp
      className={`inline-flex h-[49px] items-center justify-center rounded-xl bg-[#363535] px-6 font-serif text-[19px] text-white transition-colors hover:bg-[#4a4949] active:bg-[#252424] disabled:opacity-50 disabled:pointer-events-none ${className}`}
      {...props}
    >
      {children}
    </Comp>
  );
}

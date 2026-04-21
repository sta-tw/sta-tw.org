<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.
<!-- END:nextjs-agent-rules -->

## Design Reference

Use the Figma design as the visual reference for implementation:
https://www.figma.com/design/GCQpcLkx0PW9TOLu2X5zhJ/%E7%89%B9%E9%81%B8%E7%B6%B2%E7%AB%99?node-id=0-1&t=bGCBDhDCCLDVHFFm-1

The result should look the same as the design, but the design is a reference, not a rulebook. During implementation, always apply UX judgment and web development best practices. Do not follow the design blindly: extract reusable components where appropriate, use flexible and responsive layout primitives such as flexbox, keep the UI accessible and maintainable, and make decisions like a web developer building a production site.

Simplify frame and div structure whenever best practice calls for it. Figma frames map to DOM elements mechanically, but the HTML should reflect semantic meaning and layout intent — not the design tool's layer hierarchy. Flatten unnecessary wrapper elements, use semantic HTML tags (`<section>`, `<nav>`, `<article>`, etc.) over generic `<div>`s where appropriate, and collapse redundant nesting that exists only as a Figma organizational artifact.

Always use Radix UI primitives when possible for accessible, composable UI behavior.

When translating design values to code, normalize arbitrary numbers to the nearest semantic Tailwind utility (e.g. `h-12` instead of `h-[49px]`, `text-xl` instead of `text-[19px]`). Only use arbitrary values when no standard token is close enough to matter visually.

## File Naming

All files must use kebab-case (e.g. `my-component.tsx`, `use-auth.ts`). Component names inside the file remain PascalCase as per React convention.

## Responsiveness

Every UI component and layout must be responsive. Never produce content that is fixed-width, overflows on small screens, or breaks at any viewport size. Use Tailwind responsive prefixes (`sm:`, `md:`, `lg:`, etc.) and fluid layout primitives (flexbox, grid, `min-w-0`, etc.) to ensure the UI works from mobile to desktop.

## Development Servers

Never start development servers unless explicitly told to do so by the user.


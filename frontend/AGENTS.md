<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.
<!-- END:nextjs-agent-rules -->

## Design Reference

Use the Figma design as the visual reference for implementation:
https://www.figma.com/design/GCQpcLkx0PW9TOLu2X5zhJ/%E7%89%B9%E9%81%B8%E7%B6%B2%E7%AB%99?node-id=0-1&t=bGCBDhDCCLDVHFFm-1

The result should look the same as the design, but the design is a reference, not a rulebook. During implementation, always apply UX judgment and web development best practices. Do not follow the design blindly: extract reusable components where appropriate, use flexible and responsive layout primitives such as flexbox, keep the UI accessible and maintainable, and make decisions like a web developer building a production site.

Always use Radix UI primitives when possible for accessible, composable UI behavior.

// Mock switch. Public variables injected at build time (only NEXT_PUBLIC_-prefixed vars are readable in the browser).
// Set NEXT_PUBLIC_MOCK=1 on Vercel to run the entire site on mocks, with no backend needed.
export const MOCK = process.env.NEXT_PUBLIC_MOCK === "1";

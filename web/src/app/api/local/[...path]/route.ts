import { proxyLocalAPI } from "@/lib/server/proxy";

export const dynamic = "force-dynamic";
export const runtime = "nodejs";

function handle(request: Request) { return proxyLocalAPI(request); }
export const GET = handle;
export const POST = handle;
export const PUT = handle;
export const DELETE = handle;

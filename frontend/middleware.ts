import { NextRequest,NextResponse } from "next/server";
export function middleware(request:NextRequest){if(!request.cookies.has("access_token")&&!request.cookies.has("refresh_token")){return NextResponse.redirect(new URL("/login",request.url))}return NextResponse.next()}
export const config={matcher:["/dashboard/:path*","/print/:path*"]};

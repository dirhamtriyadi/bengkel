import { cookies } from "next/headers";
import { NextRequest, NextResponse } from "next/server";
const base=process.env.API_URL??process.env.NEXT_PUBLIC_API_URL??"http://localhost:8080/api/v1";

async function proxy(request:NextRequest,parts:string[]){
  const jar=await cookies();let access=jar.get("access_token")?.value;const rawBody=["GET","HEAD"].includes(request.method)?undefined:await request.arrayBuffer();
  const call=()=>fetch(`${base}/${parts.join("/")}${request.nextUrl.search}`,{method:request.method,headers:{"Content-Type":request.headers.get("content-type")??"application/json",Authorization:access?`Bearer ${access}`:"","X-Branch-ID":request.headers.get("x-branch-id")??""},body:rawBody,cache:"no-store"});
  let upstream=await call();
  if(upstream.status===401&&jar.get("refresh_token")?.value){const refreshed=await fetch(`${base}/auth/refresh`,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({refresh_token:jar.get("refresh_token")!.value}),cache:"no-store"});if(refreshed.ok){const payload=await refreshed.json();access=payload.data.access_token;jar.set("access_token",access!,{httpOnly:true,sameSite:"lax",secure:process.env.NODE_ENV==="production",path:"/",expires:new Date(payload.data.expires_at)});jar.set("refresh_token",payload.data.refresh_token,{httpOnly:true,sameSite:"strict",secure:process.env.NODE_ENV==="production",path:"/",maxAge:60*60*24*7});upstream=await call()}}
  return new NextResponse(await upstream.arrayBuffer(),{status:upstream.status,headers:{"Content-Type":upstream.headers.get("content-type")??"application/json","X-Request-ID":upstream.headers.get("x-request-id")??""}})
}
type Context={params:Promise<{path:string[]}>};
export async function GET(r:NextRequest,c:Context){return proxy(r,(await c.params).path)}
export async function POST(r:NextRequest,c:Context){return proxy(r,(await c.params).path)}
export async function PUT(r:NextRequest,c:Context){return proxy(r,(await c.params).path)}
export async function PATCH(r:NextRequest,c:Context){return proxy(r,(await c.params).path)}
export async function DELETE(r:NextRequest,c:Context){return proxy(r,(await c.params).path)}

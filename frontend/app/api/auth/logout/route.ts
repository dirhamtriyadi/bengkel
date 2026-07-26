import { cookies } from "next/headers";
import { NextResponse } from "next/server";
const base=process.env.API_URL??process.env.NEXT_PUBLIC_API_URL??"http://localhost:8080/api/v1";
export async function POST(){const jar=await cookies();const access=jar.get("access_token")?.value;const refresh=jar.get("refresh_token")?.value;if(access&&refresh){await fetch(`${base}/auth/logout`,{method:"POST",headers:{"Content-Type":"application/json",Authorization:`Bearer ${access}`},body:JSON.stringify({refresh_token:refresh})}).catch(()=>null)}jar.delete("access_token");jar.delete("refresh_token");return NextResponse.json({data:{message:"Berhasil keluar"}})}

import { cookies } from "next/headers";
import { NextResponse } from "next/server";
import { apiBaseURL } from "@/lib/api-base";
const base=apiBaseURL();
export async function POST(){const jar=await cookies();const access=jar.get("access_token")?.value;const refresh=jar.get("refresh_token")?.value;if(access&&refresh){await fetch(`${base}/auth/logout`,{method:"POST",headers:{"Content-Type":"application/json",Authorization:`Bearer ${access}`},body:JSON.stringify({refresh_token:refresh})}).catch(()=>null)}jar.delete("access_token");jar.delete("refresh_token");return NextResponse.json({data:{message:"Berhasil keluar"}})}

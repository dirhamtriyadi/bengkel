"use client";
import { zodResolver } from "@hookform/resolvers/zod";
import { ArrowRight, Loader2, Wrench } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Card,CardContent,CardDescription,CardHeader,CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

const schema=z.object({email:z.string().email("Email tidak valid"),password:z.string().min(8,"Minimal 8 karakter")});
type Form=z.infer<typeof schema>;
export default function LoginPage(){const router=useRouter();const [serverError,setServerError]=useState("");const {register,handleSubmit,formState:{errors,isSubmitting}}=useForm<Form>({resolver:zodResolver(schema),defaultValues:{email:"owner@bengkel.local",password:"Bengkel123!"}});
async function submit(values:Form){setServerError("");const res=await fetch("/api/auth/login",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(values)});const body=await res.json();if(!res.ok){setServerError(body.error?.message??"Login gagal");return}router.replace("/dashboard");router.refresh()}
return <main className="dot-grid grid min-h-screen place-items-center px-6"><Card className="w-full max-w-md"><CardHeader><span className="mb-5 grid size-11 place-items-center rounded-xl bg-primary text-white"><Wrench className="size-5"/></span><CardTitle className="text-2xl">Selamat datang kembali</CardTitle><CardDescription>Masuk ke pusat operasional bengkel.</CardDescription></CardHeader><CardContent><form onSubmit={handleSubmit(submit)} className="space-y-5"><div><label className="mb-2 block text-sm font-medium">Email</label><Input type="email" {...register("email")}/>{errors.email&&<p className="mt-1 text-xs text-destructive">{errors.email.message}</p>}</div><div><label className="mb-2 block text-sm font-medium">Kata sandi</label><Input type="password" {...register("password")}/>{errors.password&&<p className="mt-1 text-xs text-destructive">{errors.password.message}</p>}</div>{serverError&&<p className="rounded-xl bg-red-50 p-3 text-sm text-destructive">{serverError}</p>}<Button className="w-full" size="lg" disabled={isSubmitting}>{isSubmitting?<Loader2 className="size-4 animate-spin"/>:<>Masuk<ArrowRight className="size-4"/></>}</Button><p className="text-center text-xs text-muted-foreground">Demo: owner@bengkel.local / Bengkel123!</p></form></CardContent></Card></main>}

"use client";
import Link from "next/link";
import { usePathname,useRouter } from "next/navigation";
import { BarChart3,Boxes,ClipboardList,FileClock,LayoutDashboard,LogOut,Menu,Package,ReceiptText,Settings,ShoppingCart,Users,Wrench,X } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const navigation=[
  {href:"/dashboard",label:"Ringkasan",icon:LayoutDashboard},
  {href:"/dashboard/work-orders",label:"Work order",icon:ClipboardList},
  {href:"/dashboard/pos",label:"Kasir / POS",icon:ShoppingCart},
  {href:"/dashboard/sales",label:"Transaksi",icon:ReceiptText},
  {href:"/dashboard/customers",label:"Pelanggan",icon:Users},
  {href:"/dashboard/vehicles",label:"Kendaraan",icon:Wrench},
  {href:"/dashboard/products",label:"Produk & jasa",icon:Package},
  {href:"/dashboard/inventory",label:"Persediaan",icon:Boxes},
  {href:"/dashboard/reports",label:"Akuntansi",icon:BarChart3},
  {href:"/dashboard/audit-logs",label:"Audit log",icon:FileClock},
  {href:"/dashboard/settings",label:"Pengaturan",icon:Settings},
];

export function DashboardShell({children}:{children:React.ReactNode}){const pathname=usePathname();const router=useRouter();const [open,setOpen]=useState(false);async function logout(){await fetch("/api/auth/logout",{method:"POST"});router.replace("/login");router.refresh()}
const sidebar=<><div className="flex h-20 items-center gap-3 px-6"><span className="grid size-10 place-items-center rounded-xl bg-primary text-white"><Wrench className="size-5"/></span><div><p className="font-bold leading-tight">BengkelOS</p><p className="text-xs text-muted-foreground">Cabang Pusat</p></div></div><nav className="flex-1 space-y-1 overflow-y-auto px-3 py-3">{navigation.map(item=>{const active=item.href==="/dashboard"?pathname===item.href:pathname.startsWith(item.href);return <Link key={item.href} href={item.href} onClick={()=>setOpen(false)} className={cn("flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium text-muted-foreground transition hover:bg-muted hover:text-foreground",active&&"bg-primary/10 text-primary")}><item.icon className="size-4"/>{item.label}</Link>})}</nav><div className="border-t p-3"><button onClick={logout} className="flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium text-muted-foreground hover:bg-muted"><LogOut className="size-4"/>Keluar</button></div></>;
return <div className="min-h-screen bg-muted/40"><aside className="fixed inset-y-0 left-0 z-30 hidden w-64 flex-col border-r bg-white lg:flex">{sidebar}</aside>{open&&<div className="fixed inset-0 z-40 lg:hidden"><button aria-label="Tutup menu" className="absolute inset-0 bg-black/30" onClick={()=>setOpen(false)}/><aside className="relative flex h-full w-72 flex-col bg-white shadow-xl">{sidebar}<Button variant="ghost" size="icon" className="absolute right-3 top-5" onClick={()=>setOpen(false)}><X className="size-5"/></Button></aside></div>}<header className="sticky top-0 z-20 flex h-16 items-center justify-between border-b bg-white/90 px-5 backdrop-blur lg:ml-64 lg:px-8"><Button variant="ghost" size="icon" className="lg:hidden" onClick={()=>setOpen(true)}><Menu className="size-5"/></Button><div className="ml-auto flex items-center gap-3"><div className="hidden text-right sm:block"><p className="text-sm font-semibold">Owner Bengkel</p><p className="text-xs text-muted-foreground">owner@bengkel.local</p></div><div className="grid size-9 place-items-center rounded-full bg-foreground text-xs font-bold text-white">OB</div></div></header><main className="p-5 lg:ml-64 lg:p-8">{children}</main></div>}

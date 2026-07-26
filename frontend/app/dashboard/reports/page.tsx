"use client";

import { useQuery } from "@tanstack/react-query";
import { TrendingDown,TrendingUp,Wallet } from "lucide-react";
import { apiClient } from "@/lib/api";
import { rupiah } from "@/lib/utils";
import { Card,CardContent,CardHeader,CardTitle } from "@/components/ui/card";

type AccountRow={code:string;name:string;type:string;balance:number};
type Report={from:string;to:string;total_revenue:number;total_expense:number;net_profit:number;accounts:AccountRow[]|null};

export default function Reports(){
  const report=useQuery({queryKey:["profit-loss"],queryFn:()=>apiClient<Report>("/reports/profit-loss")});
  const data=report.data?.data;
  const accounts=data?.accounts??[];
  const stats=[
    {Icon:TrendingUp,label:"Pendapatan",value:data?.total_revenue??0,tone:"text-emerald-600"},
    {Icon:TrendingDown,label:"Beban",value:data?.total_expense??0,tone:"text-red-600"},
    {Icon:Wallet,label:"Laba bersih",value:data?.net_profit??0,tone:"text-blue-600"},
  ];

  return <div className="space-y-6">
    <div><h1 className="text-3xl font-bold tracking-tight">Akuntansi & laporan</h1><p className="mt-2 text-muted-foreground">Laba rugi berbasis jurnal double-entry yang diposting.</p></div>
    {report.isError&&<p className="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700">{report.error.message}</p>}
    <div className="grid gap-4 md:grid-cols-3">{stats.map(({Icon,label,value,tone})=><Card key={label}><CardContent className="flex items-center justify-between p-6"><div><p className="text-sm text-muted-foreground">{label}</p><p className="mt-2 text-2xl font-bold">{rupiah.format(value)}</p></div><Icon className={`size-7 ${tone}`}/></CardContent></Card>)}</div>
    <Card><CardHeader><CardTitle>Rincian laba rugi</CardTitle></CardHeader><CardContent><div className="overflow-x-auto"><table className="w-full text-sm"><thead><tr className="border-b text-left"><th className="py-3">Kode</th><th>Akun</th><th>Tipe</th><th className="text-right">Saldo</th></tr></thead><tbody>{accounts.map(row=><tr className="border-b" key={row.code}><td className="py-3 font-mono">{row.code}</td><td>{row.name}</td><td>{row.type}</td><td className="text-right font-semibold">{rupiah.format(row.balance)}</td></tr>)}{!report.isLoading&&accounts.length===0&&<tr><td colSpan={4} className="py-10 text-center text-muted-foreground">Belum ada jurnal terposting pada periode ini.</td></tr>}</tbody></table></div></CardContent></Card>
  </div>
}

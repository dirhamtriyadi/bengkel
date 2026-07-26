"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation,useQuery,useQueryClient } from "@tanstack/react-query";
import { PackagePlus } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { DataTable } from "@/components/data-table";
import { Button } from "@/components/ui/button";
import { Dialog,DialogContent,DialogDescription,DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { apiClient } from "@/lib/api";
import { dateTime,rupiah } from "@/lib/utils";

type Product={id:string;sku:string;name:string;type:string;cost_price:number};
const schema=z.object({product_id:z.string().uuid(),direction:z.enum(["in","out","adjustment"]),quantity:z.coerce.number().positive(),unit_cost:z.coerce.number().min(0),notes:z.string().max(500)});
type Form=z.infer<typeof schema>;

export default function InventoryPage(){
 const client=useQueryClient();const [open,setOpen]=useState(false);const products=useQuery({queryKey:["inventory-products"],queryFn:()=>apiClient<Product[]>("/products?per_page=100&sort=name&direction=asc")});const {register,handleSubmit,reset,formState:{errors}}=useForm<Form>({resolver:zodResolver(schema),defaultValues:{product_id:"",direction:"in",quantity:1,unit_cost:0,notes:""}});
 const mutation=useMutation({mutationFn:(values:Form)=>apiClient("/inventory/adjustments",{method:"POST",body:JSON.stringify(values)}),onSuccess:()=>{client.invalidateQueries({queryKey:["/inventory"]});client.invalidateQueries({queryKey:["/inventory/movements"]});setOpen(false);reset()}});
 return <div className="space-y-6"><div><h1 className="text-3xl font-bold tracking-tight">Persediaan</h1><p className="mt-2 text-muted-foreground">Saldo stok dan kartu pergerakan barang per cabang.</p></div><DataTable endpoint="/inventory" columns={[{key:"sku",label:"SKU"},{key:"name",label:"Produk"},{key:"quantity",label:"Stok"},{key:"unit",label:"Satuan"},{key:"min_stock",label:"Minimum"},{key:"updated_at",label:"Diperbarui",format:value=>dateTime.format(new Date(String(value)))}]} toolbar={<Button onClick={()=>setOpen(true)}><PackagePlus className="size-4"/>Penyesuaian stok</Button>}/><div><h2 className="mb-3 text-xl font-bold">Kartu stok</h2><DataTable endpoint="/inventory/movements" columns={[{key:"created_at",label:"Waktu",format:value=>dateTime.format(new Date(String(value)))},{key:"sku",label:"SKU"},{key:"product_name",label:"Produk"},{key:"direction",label:"Arah"},{key:"quantity",label:"Jumlah"},{key:"unit_cost",label:"Biaya",format:value=>rupiah.format(Number(value))},{key:"reference_type",label:"Sumber"},{key:"notes",label:"Catatan"}]}/></div><Dialog open={open} onOpenChange={setOpen}><DialogContent><DialogTitle>Penyesuaian stok</DialogTitle><DialogDescription>Gunakan masuk/keluar untuk mutasi, atau adjustment untuk menetapkan saldo akhir.</DialogDescription><form className="mt-6 space-y-4" onSubmit={handleSubmit(values=>mutation.mutate(values))}><label className="block text-sm font-medium">Barang<select className="mt-2 h-10 w-full rounded-xl border bg-white px-3" {...register("product_id")}><option value="">Pilih barang</option>{products.data?.data?.filter(product=>product.type==="part").map(product=><option key={product.id} value={product.id}>{product.sku} · {product.name}</option>)}</select>{errors.product_id&&<span className="text-xs text-red-600">Pilih barang</span>}</label><div className="grid grid-cols-2 gap-4"><label className="text-sm font-medium">Jenis<select className="mt-2 h-10 w-full rounded-xl border bg-white px-3" {...register("direction")}><option value="in">Barang masuk</option><option value="out">Barang keluar</option><option value="adjustment">Saldo akhir</option></select></label><label className="text-sm font-medium">Jumlah<Input className="mt-2" type="number" step=".001" {...register("quantity",{valueAsNumber:true})}/></label></div><label className="block text-sm font-medium">Harga modal satuan<Input className="mt-2" type="number" {...register("unit_cost",{valueAsNumber:true})}/></label><label className="block text-sm font-medium">Catatan<Input className="mt-2" {...register("notes")}/></label>{mutation.error&&<p className="text-sm text-red-600">{mutation.error.message}</p>}<div className="flex justify-end gap-2"><Button type="button" variant="outline" onClick={()=>setOpen(false)}>Batal</Button><Button disabled={mutation.isPending}>Simpan mutasi</Button></div></form></DialogContent></Dialog></div>
}

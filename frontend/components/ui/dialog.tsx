"use client";

import * as DialogPrimitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

export const Dialog=DialogPrimitive.Root;
export const DialogTrigger=DialogPrimitive.Trigger;
export const DialogClose=DialogPrimitive.Close;

export function DialogContent({children,className}:{children:React.ReactNode;className?:string}){
  return <DialogPrimitive.Portal>
    <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/40 backdrop-blur-sm"/>
    <DialogPrimitive.Content className={cn("fixed left-1/2 top-1/2 z-50 max-h-[90vh] w-[calc(100%-2rem)] max-w-2xl -translate-x-1/2 -translate-y-1/2 overflow-y-auto rounded-2xl border bg-white p-6 shadow-2xl",className)}>
      {children}
      <DialogPrimitive.Close className="absolute right-4 top-4 rounded-lg p-2 text-muted-foreground hover:bg-muted"><X className="size-4"/></DialogPrimitive.Close>
    </DialogPrimitive.Content>
  </DialogPrimitive.Portal>
}
export const DialogTitle=({children}:{children:React.ReactNode})=><DialogPrimitive.Title className="text-xl font-bold">{children}</DialogPrimitive.Title>;
export const DialogDescription=({children}:{children:React.ReactNode})=><DialogPrimitive.Description className="mt-1 text-sm text-muted-foreground">{children}</DialogPrimitive.Description>;

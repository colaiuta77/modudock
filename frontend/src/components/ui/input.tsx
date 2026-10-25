// 공통 입력 필드의 스타일과 접근성 상태를 제공한다.
import * as React from "react";
import { cn } from "@/lib/utils";
import { formFieldBase, formFieldSingleLine } from "./_shared";

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        formFieldBase,
        formFieldSingleLine,
        "selection:bg-primary selection:text-primary-foreground file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground",
        className,
      )}
      {...props}
    />
  );
}

export { Input };

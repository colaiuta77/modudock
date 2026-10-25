// 공통 여러 줄 입력 필드의 스타일을 제공한다.
import * as React from "react";
import { cn } from "@/lib/utils";
import { formFieldBase, formFieldMultiLine } from "./_shared";

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        formFieldBase,
        formFieldMultiLine,
        className,
      )}
      {...props}
    />
  );
}

export { Textarea };

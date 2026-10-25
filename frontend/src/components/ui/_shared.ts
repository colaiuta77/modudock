// Precision 입력 컴포넌트가 공유하는 테두리·포커스·비활성 스타일을 정의한다.
// Adapted from Better Design Precision (MIT).
export const formFieldBase =
  "w-full min-w-0 rounded-md px-3 text-base md:text-sm bg-background text-foreground border border-input placeholder:text-muted-foreground transition-[border-color,box-shadow] duration-150 ease-out hover:enabled:border-ring/45 focus-visible:outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/25 focus-visible:ring-offset-0 disabled:cursor-not-allowed disabled:opacity-50 disabled:bg-muted aria-invalid:border-destructive aria-invalid:focus-visible:ring-destructive/25";
export const formFieldSingleLine = "flex h-9 py-1.5";
export const formFieldMultiLine = "flex field-sizing-content min-h-20 py-2 resize-y";

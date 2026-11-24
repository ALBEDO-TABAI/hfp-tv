import { motion, type HTMLMotionProps } from "motion/react";
import { forwardRef, type ReactNode } from "react";

import { cn } from "@/lib/utils";

type SpotlightSurfaceProps = Omit<HTMLMotionProps<"div">, "children"> & {
    children?: ReactNode;
    spotlightColor: string;
    spotlightRadius?: number;
    contentClassName?: string;
};

export const SpotlightSurface = forwardRef<HTMLDivElement, SpotlightSurfaceProps>(function SpotlightSurface(
    { children, className, contentClassName, spotlightColor: _spotlightColor, spotlightRadius: _spotlightRadius, ...props },
    ref,
) {
    return (
        <motion.div ref={ref} className={cn("relative isolate", className)} {...props}>
            <div className={cn(contentClassName ?? "relative z-[1] flex min-h-0 flex-1 flex-col")}>{children}</div>
        </motion.div>
    );
});

import { cn } from "@/lib/utils"

type SparklineProps = {
  data: number[]
  className?: string
  tone?: "info" | "success" | "warning" | "destructive"
  width?: number
  height?: number
}

const toneVar: Record<NonNullable<SparklineProps["tone"]>, string> = {
  info: "var(--info)",
  success: "var(--success)",
  warning: "var(--warning)",
  destructive: "var(--destructive)",
}

export function Sparkline({
  data,
  className,
  tone = "info",
  width = 72,
  height = 24,
}: SparklineProps) {
  const max = Math.max(...data, 100)
  const min = Math.min(...data, 0)
  const range = max - min || 1
  const step = width / (data.length - 1)

  const points = data.map((v, i) => {
    const x = i * step
    const y = height - ((v - min) / range) * height
    return [x, y] as const
  })

  const line = points.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(" ")
  const area = `0,${height} ${line} ${width},${height}`
  const color = toneVar[tone]
  const gid = `spark-${tone}`

  return (
    <svg
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      className={cn("overflow-visible", className)}
      aria-hidden="true"
    >
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={color} stopOpacity="0.35" />
          <stop offset="100%" stopColor={color} stopOpacity="0" />
        </linearGradient>
      </defs>
      <polygon points={area} fill={`url(#${gid})`} />
      <polyline
        points={line}
        fill="none"
        stroke={color}
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

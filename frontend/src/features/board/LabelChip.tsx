interface LabelChipProps {
  name: string
  color: string
  onRemove?: () => void
}

export default function LabelChip({ name, color, onRemove }: LabelChipProps) {
  return (
    <span
      className="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium text-white"
      style={{ backgroundColor: color }}
    >
      {name}
      {onRemove && (
        <button
          type="button"
          onClick={onRemove}
          aria-label={`Remover etiqueta ${name}`}
          className="leading-none text-white/80 hover:text-white"
        >
          ✕
        </button>
      )}
    </span>
  )
}

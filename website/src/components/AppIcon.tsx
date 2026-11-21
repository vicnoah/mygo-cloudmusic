// The app's icon, shown as an <img> so it always matches resources/icon.png.
export function AppIcon(props: { size?: number; class?: string }) {
  const s = props.size ?? 32
  return (
    <img
      class={props.class}
      src={import.meta.env.BASE_URL + 'icon.png'}
      width={s}
      height={s}
      alt="Wagyu Music"
      aria-hidden="true"
      style={{ 'border-radius': `${Math.round(s * 0.22)}px` }}
    />
  )
}

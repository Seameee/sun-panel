const svgNamespace = 'http://www.w3.org/2000/svg'
const icons = import.meta.glob<string>('../assets/svg-icons/*.svg', {
  eager: true,
  import: 'default',
  query: '?raw',
})

export function registerLocalIcons() {
  const sprite = document.createElementNS(svgNamespace, 'svg')
  sprite.setAttribute('aria-hidden', 'true')
  sprite.style.display = 'none'
  const parser = new DOMParser()

  for (const [path, content] of Object.entries(icons)) {
    const icon = parser.parseFromString(content, 'image/svg+xml').documentElement
    if (icon.namespaceURI !== svgNamespace || icon.localName !== 'svg')
      continue
    const name = path.slice(path.lastIndexOf('/') + 1, -4)
    const symbol = document.createElementNS(svgNamespace, 'symbol')
    symbol.id = name
    symbol.setAttribute('viewBox', icon.getAttribute('viewBox') ?? '0 0 24 24')
    for (const child of icon.children)
      symbol.append(document.importNode(child, true))
    sprite.append(symbol)
  }

  document.body.prepend(sprite)
}

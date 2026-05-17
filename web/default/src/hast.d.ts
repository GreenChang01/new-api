declare module 'hast' {
  export interface Element {
    type: 'element'
    tagName: string
    properties?: Record<string, unknown>
    children: Array<Element | Text>
  }

  export interface Text {
    type: 'text'
    value: string
  }
}

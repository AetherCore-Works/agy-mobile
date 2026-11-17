export type IconName =
  | 'menu'
  | 'plus'
  | 'terminal'
  | 'inbox'
  | 'outbox'
  | 'paperclip'
  | 'send'
  | 'mic'
  | 'settings'
  | 'search'
  | 'trash'
  | 'download'
  | 'refresh'
  | 'close'
  | 'shield'
  | 'zap'
  | 'eye'
  | 'power'
  | 'cpu'
  | 'cloud'
  | 'clipboard'
  | 'chevron-up'
  | 'chevron-down'
  | 'check'
  | 'alert'
  | 'file'
  | 'clock'
  | 'stop'
  | 'sun'
  | 'moon'
  | 'palette'
  | 'sparkles'
  | 'pin'
  | 'edit'
  | 'share'
  | 'columns'
  | 'compare'
  | 'tag'
  | 'link';

export interface IconShape {
  tag: 'path' | 'circle' | 'rect' | 'line' | 'polyline' | 'polygon';
  attrs: Record<string, string | number>;
}

const ICONS: Record<IconName, IconShape[]> = {
  menu: [
    { tag: 'line', attrs: { x1: 4, y1: 6, x2: 20, y2: 6 } },
    { tag: 'line', attrs: { x1: 4, y1: 12, x2: 20, y2: 12 } },
    { tag: 'line', attrs: { x1: 4, y1: 18, x2: 20, y2: 18 } },
  ],
  plus: [
    { tag: 'line', attrs: { x1: 12, y1: 5, x2: 12, y2: 19 } },
    { tag: 'line', attrs: { x1: 5, y1: 12, x2: 19, y2: 12 } },
  ],
  terminal: [
    { tag: 'polyline', attrs: { points: '4 17 10 11 4 5' } },
    { tag: 'line', attrs: { x1: 12, y1: 19, x2: 20, y2: 19 } },
  ],
  inbox: [
    { tag: 'polyline', attrs: { points: '22 12 16 12 14 15 10 15 8 12 2 12' } },
    { tag: 'path', attrs: { d: 'M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z' } },
  ],
  outbox: [
    { tag: 'path', attrs: { d: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4' } },
    { tag: 'polyline', attrs: { points: '7 10 12 15 17 10' } },
    { tag: 'line', attrs: { x1: 12, y1: 15, x2: 12, y2: 3 } },
  ],
  paperclip: [
    { tag: 'path', attrs: { d: 'M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48' } },
  ],
  send: [
    { tag: 'line', attrs: { x1: 22, y1: 2, x2: 11, y2: 13 } },
    { tag: 'polygon', attrs: { points: '22 2 15 22 11 13 2 9 22 2' } },
  ],
  mic: [
    { tag: 'path', attrs: { d: 'M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z' } },
    { tag: 'path', attrs: { d: 'M19 10v2a7 7 0 0 1-14 0v-2' } },
    { tag: 'line', attrs: { x1: 12, y1: 19, x2: 12, y2: 23 } },
    { tag: 'line', attrs: { x1: 8, y1: 23, x2: 16, y2: 23 } },
  ],
  settings: [
    { tag: 'circle', attrs: { cx: 12, cy: 12, r: 3 } },
    { tag: 'path', attrs: { d: 'M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z' } },
  ],
  search: [
    { tag: 'circle', attrs: { cx: 11, cy: 11, r: 8 } },
    { tag: 'line', attrs: { x1: 21, y1: 21, x2: 16.65, y2: 16.65 } },
  ],
  trash: [
    { tag: 'polyline', attrs: { points: '3 6 5 6 21 6' } },
    { tag: 'path', attrs: { d: 'M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2' } },
  ],
  download: [
    { tag: 'path', attrs: { d: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4' } },
    { tag: 'polyline', attrs: { points: '7 10 12 15 17 10' } },
    { tag: 'line', attrs: { x1: 12, y1: 15, x2: 12, y2: 3 } },
  ],
  refresh: [
    { tag: 'polyline', attrs: { points: '23 4 23 10 17 10' } },
    { tag: 'polyline', attrs: { points: '1 20 1 14 7 14' } },
    { tag: 'path', attrs: { d: 'M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15' } },
  ],
  close: [
    { tag: 'line', attrs: { x1: 18, y1: 6, x2: 6, y2: 18 } },
    { tag: 'line', attrs: { x1: 6, y1: 6, x2: 18, y2: 18 } },
  ],
  shield: [
    { tag: 'path', attrs: { d: 'M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z' } },
  ],
  zap: [
    { tag: 'polygon', attrs: { points: '13 2 3 14 12 14 11 22 21 10 12 10 13 2' } },
  ],
  eye: [
    { tag: 'path', attrs: { d: 'M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z' } },
    { tag: 'circle', attrs: { cx: 12, cy: 12, r: 3 } },
  ],
  power: [
    { tag: 'path', attrs: { d: 'M18.36 6.64a9 9 0 1 1-12.73 0' } },
    { tag: 'line', attrs: { x1: 12, y1: 2, x2: 12, y2: 12 } },
  ],
  cpu: [
    { tag: 'rect', attrs: { x: 4, y: 4, width: 16, height: 16, rx: 2, ry: 2 } },
    { tag: 'rect', attrs: { x: 9, y: 9, width: 6, height: 6 } },
    { tag: 'line', attrs: { x1: 9, y1: 1, x2: 9, y2: 4 } },
    { tag: 'line', attrs: { x1: 15, y1: 1, x2: 15, y2: 4 } },
    { tag: 'line', attrs: { x1: 9, y1: 20, x2: 9, y2: 23 } },
    { tag: 'line', attrs: { x1: 15, y1: 20, x2: 15, y2: 23 } },
    { tag: 'line', attrs: { x1: 20, y1: 9, x2: 23, y2: 9 } },
    { tag: 'line', attrs: { x1: 20, y1: 14, x2: 23, y2: 14 } },
    { tag: 'line', attrs: { x1: 1, y1: 9, x2: 4, y2: 9 } },
    { tag: 'line', attrs: { x1: 1, y1: 14, x2: 4, y2: 14 } },
  ],
  cloud: [
    { tag: 'path', attrs: { d: 'M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z' } },
  ],
  clipboard: [
    { tag: 'path', attrs: { d: 'M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2' } },
    { tag: 'rect', attrs: { x: 8, y: 2, width: 8, height: 4, rx: 1, ry: 1 } },
  ],
  'chevron-up': [
    { tag: 'polyline', attrs: { points: '18 15 12 9 6 15' } },
  ],
  'chevron-down': [
    { tag: 'polyline', attrs: { points: '6 9 12 15 18 9' } },
  ],
  check: [
    { tag: 'polyline', attrs: { points: '20 6 9 17 4 12' } },
  ],
  alert: [
    { tag: 'circle', attrs: { cx: 12, cy: 12, r: 10 } },
    { tag: 'line', attrs: { x1: 12, y1: 8, x2: 12, y2: 12 } },
    { tag: 'line', attrs: { x1: 12, y1: 16, x2: 12.01, y2: 16 } },
  ],
  file: [
    { tag: 'path', attrs: { d: 'M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z' } },
    { tag: 'polyline', attrs: { points: '13 2 13 9 20 9' } },
  ],
  clock: [
    { tag: 'circle', attrs: { cx: 12, cy: 12, r: 10 } },
    { tag: 'polyline', attrs: { points: '12 6 12 12 16 14' } },
  ],
  stop: [
    { tag: 'rect', attrs: { x: 6, y: 6, width: 12, height: 12, rx: 2, ry: 2 } },
  ],
  sun: [
    { tag: 'circle', attrs: { cx: 12, cy: 12, r: 4 } },
    { tag: 'line', attrs: { x1: 12, y1: 2, x2: 12, y2: 4 } },
    { tag: 'line', attrs: { x1: 12, y1: 20, x2: 12, y2: 22 } },
    { tag: 'line', attrs: { x1: 4.93, y1: 4.93, x2: 6.34, y2: 6.34 } },
    { tag: 'line', attrs: { x1: 17.66, y1: 17.66, x2: 19.07, y2: 19.07 } },
    { tag: 'line', attrs: { x1: 2, y1: 12, x2: 4, y2: 12 } },
    { tag: 'line', attrs: { x1: 20, y1: 12, x2: 22, y2: 12 } },
    { tag: 'line', attrs: { x1: 6.34, y1: 17.66, x2: 4.93, y2: 19.07 } },
    { tag: 'line', attrs: { x1: 19.07, y1: 4.93, x2: 17.66, y2: 6.34 } },
  ],
  moon: [
    { tag: 'path', attrs: { d: 'M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z' } },
  ],
  palette: [
    { tag: 'path', attrs: { d: 'M12 22a1 1 0 0 1 0-20 10 9 0 0 1 10 9 5 5 0 0 1-5 5h-2.25a1.75 1.75 0 0 0-1.4 2.8l.3.4a1.75 1.75 0 0 1-1.4 2.8z' } },
    { tag: 'circle', attrs: { cx: 13.5, cy: 6.5, r: 0.9 } },
    { tag: 'circle', attrs: { cx: 17.5, cy: 10.5, r: 0.9 } },
    { tag: 'circle', attrs: { cx: 8.5, cy: 7.5, r: 0.9 } },
    { tag: 'circle', attrs: { cx: 6.5, cy: 12.5, r: 0.9 } },
  ],
  sparkles: [
    { tag: 'path', attrs: { d: 'M12 3l1.9 5.1L19 10l-5.1 1.9L12 17l-1.9-5.1L5 10l5.1-1.9L12 3z' } },
    { tag: 'path', attrs: { d: 'M19 15l.9 2.1L22 18l-2.1.9L19 21l-.9-2.1L16 18l2.1-.9L19 15z' } },
  ],
  pin: [
    { tag: 'path', attrs: { d: 'M12 17v5' } },
    { tag: 'path', attrs: { d: 'M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z' } },
  ],
  edit: [
    { tag: 'path', attrs: { d: 'M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7' } },
    { tag: 'path', attrs: { d: 'M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z' } },
  ],
  share: [
    { tag: 'circle', attrs: { cx: 18, cy: 5, r: 3 } },
    { tag: 'circle', attrs: { cx: 6, cy: 12, r: 3 } },
    { tag: 'circle', attrs: { cx: 18, cy: 19, r: 3 } },
    { tag: 'line', attrs: { x1: 8.59, y1: 13.51, x2: 15.42, y2: 17.49 } },
    { tag: 'line', attrs: { x1: 15.41, y1: 6.51, x2: 8.59, y2: 10.49 } },
  ],
  columns: [
    { tag: 'rect', attrs: { x: 3, y: 3, width: 18, height: 18, rx: 2, ry: 2 } },
    { tag: 'line', attrs: { x1: 12, y1: 3, x2: 12, y2: 21 } },
  ],
  compare: [
    { tag: 'polyline', attrs: { points: '16 3 21 3 21 8' } },
    { tag: 'line', attrs: { x1: 4, y1: 20, x2: 21, y2: 3 } },
    { tag: 'polyline', attrs: { points: '21 16 21 21 16 21' } },
    { tag: 'line', attrs: { x1: 15, y1: 15, x2: 21, y2: 21 } },
    { tag: 'line', attrs: { x1: 4, y1: 4, x2: 9, y2: 9 } },
  ],
  tag: [
    { tag: 'path', attrs: { d: 'M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z' } },
    { tag: 'line', attrs: { x1: 7, y1: 7, x2: 7.01, y2: 7 } },
  ],
  link: [
    { tag: 'path', attrs: { d: 'M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71' } },
    { tag: 'path', attrs: { d: 'M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71' } },
  ],
};

export function resolveIconShapes(name: IconName): IconShape[] {
  return ICONS[name] ?? [];
}

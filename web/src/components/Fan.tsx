/**
 * Leque de gerações — assinatura visual do Worrel.
 * Cada raio é uma geração de skill radiando de um ponto de origem.
 * `mark` = versão compacta para o logo; `hero` = leque amplo p/ estados vazios;
 * `FanLineage` = leque vivo onde cada raio é uma geração REAL, colorida por tipo.
 */

const WARM = ['#EE2E96', '#FF6A1A', '#FFC02E', '#2FA4EE', '#1F9D57'];

// cor por tipo de evolução — espelha os pills de tipo no resto do app.
const TYPE_COLOR: Record<string, string> = {
  correction: '#EE2E96',
  learned: '#2FA4EE',
  variant: '#FFC02E',
};

export interface FanGen {
  generation: number;
  evolution_type: string;
  active?: boolean;
}

/**
 * Leque vivo: N gerações viram N raios, abrindo num arco. O raio ativo é mais
 * grosso, ganha um ponto na ponta e o rótulo da geração. Cada raio "abre" do
 * pivô com stagger (respeitando prefers-reduced-motion via CSS).
 */
export function FanLineage({ gens, width = 300, height = 168 }: { gens: FanGen[]; width?: number; height?: number }) {
  const cx = width / 2;
  const cy = height - 10;
  const len = height - 26;
  const n = gens.length;
  // arco total de até 150°, centralizado na vertical; cresce com a contagem.
  const spread = Math.min(150, 30 + n * 18);
  const start = -spread / 2;
  const stepDeg = n > 1 ? spread / (n - 1) : 0;

  return (
    <svg
      className="fan-lineage"
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      fill="none"
      role="img"
      aria-label={`Leque de ${n} gerações`}
    >
      {gens.map((g, i) => {
        const a = (n === 1 ? 0 : start + stepDeg * i) * Math.PI / 180;
        const tx = cx + Math.sin(a) * len;
        const ty = cy - Math.cos(a) * len;
        const color = TYPE_COLOR[g.evolution_type] ?? WARM[i % WARM.length];
        return (
          <g key={g.generation} className="fan-ray" style={{ ['--d' as string]: `${i * 70}ms`, transformOrigin: `${cx}px ${cy}px` }}>
            <line
              x1={cx} y1={cy} x2={tx} y2={ty}
              stroke={color}
              strokeWidth={g.active ? 6 : 3.5}
              strokeLinecap="round"
              opacity={g.active ? 1 : 0.55}
            />
            {g.active && (
              <>
                <circle cx={tx} cy={ty} r={5.5} fill={color} />
                <text x={tx} y={ty - 12} fill={color} fontSize="11" fontWeight="700"
                  textAnchor="middle" fontFamily="'Inter Tight Variable', sans-serif">
                  gen {g.generation}
                </text>
              </>
            )}
          </g>
        );
      })}
    </svg>
  );
}

export function FanMark({ size = 22 }: { size?: number }) {
  // pivô no canto inferior esquerdo; leque abrindo para cima e à direita
  const cx = 4;
  const cy = size - 4;
  const angles = [8, 30, 52, 74, 96]; // graus medidos da vertical (0 = para cima)
  const len = size - 6;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} fill="none" aria-hidden>
      {angles.map((a, i) => {
        const rad = (a * Math.PI) / 180;
        const tx = cx + Math.sin(rad) * len;
        const ty = cy - Math.cos(rad) * len;
        return (
          <line key={i} x1={cx} y1={cy} x2={tx} y2={ty}
            stroke={WARM[i]} strokeWidth={3} strokeLinecap="round" />
        );
      })}
    </svg>
  );
}

export function FanHero({ width = 120, height = 64 }: { width?: number; height?: number }) {
  const cx = width / 2;
  const cy = height - 4;
  const angles = [-72, -54, -36, -18, 0, 18, 36, 54, 72];
  const len = height - 8;
  return (
    <svg className="fan" width={width} height={height} viewBox={`0 0 ${width} ${height}`} fill="none" aria-hidden>
      {angles.map((a, i) => {
        const rad = (a - 90) * Math.PI / 180;
        const tx = cx + Math.cos(rad) * len;
        const ty = cy + Math.sin(rad) * len;
        return (
          <line
            key={i}
            x1={cx}
            y1={cy}
            x2={tx}
            y2={ty}
            stroke={WARM[i % WARM.length]}
            strokeWidth={4}
            strokeLinecap="round"
            opacity={0.92}
          />
        );
      })}
    </svg>
  );
}

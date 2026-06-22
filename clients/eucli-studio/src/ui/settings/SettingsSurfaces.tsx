import * as React from 'react'
import { Box, Paper, type SxProps, type Theme } from '@mui/material'

type SurfaceTone = 'default' | 'muted' | 'selected' | 'danger' | 'info'

type SurfaceProps = {
  children: React.ReactNode
  sx?: SxProps<Theme>
  style?: React.CSSProperties
}

type SectionProps = SurfaceProps & {
  tone?: SurfaceTone
}

function sxList(sx?: SxProps<Theme>) {
  if (!sx) return []
  return Array.isArray(sx) ? sx : [sx]
}

function toneBackground(tone: SurfaceTone) {
  if (tone === 'selected') return 'rgba(59,130,246,.10)'
  if (tone === 'danger') return 'rgba(239,68,68,.08)'
  if (tone === 'info') return 'rgba(14,165,233,.09)'
  if (tone === 'muted') return 'rgba(248,250,252,.88)'
  return 'rgba(255,255,255,.82)'
}

const settingsFormControlSx = {
  '& .MuiOutlinedInput-root': {
    borderRadius: 2,
    bgcolor: 'rgba(255,255,255,.72)',
    boxShadow: '0 8px 22px rgba(15,23,42,.045)',
    transition: 'background-color .16s ease, box-shadow .16s ease',
    '& .MuiOutlinedInput-notchedOutline': { border: 0 },
    '&:hover': {
      bgcolor: 'rgba(255,255,255,.92)',
      boxShadow: '0 10px 26px rgba(15,23,42,.065)',
    },
    '&:hover .MuiOutlinedInput-notchedOutline': { border: 0 },
    '&.Mui-focused': {
      bgcolor: 'rgba(239,246,255,.96)',
      boxShadow: '0 12px 30px rgba(37,99,235,.10)',
    },
    '&.Mui-focused .MuiOutlinedInput-notchedOutline': { border: 0 },
    '&.Mui-disabled': {
      bgcolor: 'rgba(241,245,249,.74)',
      boxShadow: 'none',
    },
    '&.Mui-disabled .MuiOutlinedInput-notchedOutline': { border: 0 },
  },
  '& .MuiInputLabel-root': {
    color: 'text.secondary',
    fontWeight: 700,
  },
  '& .MuiInputLabel-root.Mui-focused': {
    color: 'primary.main',
  },
  '& .MuiFormHelperText-root': {
    mx: 0.5,
  },
  '& .MuiSelect-icon': {
    color: 'text.secondary',
  },
}

export function SettingsSurface(props: SurfaceProps) {
  return (
    <Paper
      elevation={0}
      sx={[
        {
          p: { xs: 1.5, sm: 1.75 },
          borderRadius: 3,
          bgcolor: 'rgba(255,255,255,.86)',
          backgroundImage: 'linear-gradient(135deg, rgba(255,255,255,.96), rgba(248,250,252,.82))',
          boxShadow: '0 18px 52px rgba(15,23,42,.09)',
          ...settingsFormControlSx,
        },
        ...sxList(props.sx),
      ]}
    >
      {props.children}
    </Paper>
  )
}

export const SettingsSection = React.forwardRef<HTMLDivElement, SectionProps>(function SettingsSection(props, ref) {
  const tone = props.tone || 'default'
  return (
    <Box
      ref={ref}
      style={props.style}
      sx={[
        {
          p: { xs: 1.25, sm: 1.5 },
          borderRadius: 2.5,
          bgcolor: toneBackground(tone),
          boxShadow: tone === 'selected' ? '0 12px 28px rgba(37,99,235,.10)' : '0 10px 24px rgba(15,23,42,.055)',
          ...settingsFormControlSx,
        },
        ...sxList(props.sx),
      ]}
    >
      {props.children}
    </Box>
  )
})

export const SettingsListItem = React.forwardRef<HTMLDivElement, SectionProps>(function SettingsListItem(props, ref) {
  return <SettingsSection ref={ref} tone={props.tone || 'default'} sx={[{ p: 1.25 }, ...sxList(props.sx)]}>{props.children}</SettingsSection>
})

export function SettingsPill(props: { children: React.ReactNode; tone?: SurfaceTone; sx?: SxProps<Theme> }) {
  return (
    <Box
      component="span"
      sx={[
        {
          display: 'inline-flex',
          alignItems: 'center',
          minHeight: 24,
          px: 1,
          borderRadius: 999,
          bgcolor: toneBackground(props.tone || 'muted'),
          color: props.tone === 'danger' ? 'error.main' : props.tone === 'selected' ? 'primary.main' : 'text.secondary',
          fontSize: 12,
          fontWeight: 800,
          lineHeight: 1,
        },
        ...sxList(props.sx),
      ]}
    >
      {props.children}
    </Box>
  )
}

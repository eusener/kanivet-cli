# Mantém o estilo fade nas abas e desenha o botão "+" estreito (3 células).
from kitty.tab_bar import draw_tab_with_fade


def draw_tab(draw_data, screen, tab, before, max_tab_length, index, is_last, extra_data):
    if tab.tab_id < 0:  # botão "+" (aba sintética do kitty)
        screen.draw(' + ')
        return screen.cursor.x
    return draw_tab_with_fade(draw_data, screen, tab, before, max_tab_length, index, is_last, extra_data)

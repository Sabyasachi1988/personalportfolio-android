package com.saby.personalportfolio

import android.content.Context
import androidx.core.content.ContextCompat

object ChartColors {
    fun palette(context: Context): List<Int> = listOf(
        ContextCompat.getColor(context, R.color.chartSlice1),
        ContextCompat.getColor(context, R.color.chartSlice2),
        ContextCompat.getColor(context, R.color.chartSlice3),
        ContextCompat.getColor(context, R.color.chartSlice4),
        ContextCompat.getColor(context, R.color.chartSlice5),
        ContextCompat.getColor(context, R.color.chartSlice6),
        ContextCompat.getColor(context, R.color.chartSlice7)
    )
}

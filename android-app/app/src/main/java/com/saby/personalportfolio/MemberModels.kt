package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

data class Member(
    @SerializedName("ID") val id: String,
    @SerializedName("Name") val name: String
)

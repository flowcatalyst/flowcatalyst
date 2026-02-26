<?php

namespace FlowCatalyst\Generated\Model;

class ApiPublicLoginThemeGetResponse200 extends \ArrayObject
{
    /**
     * @var array
     */
    protected $initialized = [];
    public function isInitialized($property): bool
    {
        return array_key_exists($property, $this->initialized);
    }
    /**
     * @var mixed|null
     */
    protected $brandName;
    /**
     * @var mixed|null
     */
    protected $brandSubtitle;
    /**
     * @var mixed|null
     */
    protected $logoUrl;
    /**
     * @var mixed|null
     */
    protected $logoSvg;
    /**
     * @var mixed|null
     */
    protected $logoHeight;
    /**
     * @var mixed|null
     */
    protected $primaryColor;
    /**
     * @var mixed|null
     */
    protected $accentColor;
    /**
     * @var mixed|null
     */
    protected $backgroundColor;
    /**
     * @var mixed|null
     */
    protected $backgroundGradient;
    /**
     * @var mixed|null
     */
    protected $footerText;
    /**
     * @var mixed|null
     */
    protected $customCss;
    /**
     * @return mixed
     */
    public function getBrandName()
    {
        return $this->brandName;
    }
    /**
     * @param mixed $brandName
     *
     * @return self
     */
    public function setBrandName($brandName): self
    {
        $this->initialized['brandName'] = true;
        $this->brandName = $brandName;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getBrandSubtitle()
    {
        return $this->brandSubtitle;
    }
    /**
     * @param mixed $brandSubtitle
     *
     * @return self
     */
    public function setBrandSubtitle($brandSubtitle): self
    {
        $this->initialized['brandSubtitle'] = true;
        $this->brandSubtitle = $brandSubtitle;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getLogoUrl()
    {
        return $this->logoUrl;
    }
    /**
     * @param mixed $logoUrl
     *
     * @return self
     */
    public function setLogoUrl($logoUrl): self
    {
        $this->initialized['logoUrl'] = true;
        $this->logoUrl = $logoUrl;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getLogoSvg()
    {
        return $this->logoSvg;
    }
    /**
     * @param mixed $logoSvg
     *
     * @return self
     */
    public function setLogoSvg($logoSvg): self
    {
        $this->initialized['logoSvg'] = true;
        $this->logoSvg = $logoSvg;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getLogoHeight()
    {
        return $this->logoHeight;
    }
    /**
     * @param mixed $logoHeight
     *
     * @return self
     */
    public function setLogoHeight($logoHeight): self
    {
        $this->initialized['logoHeight'] = true;
        $this->logoHeight = $logoHeight;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getPrimaryColor()
    {
        return $this->primaryColor;
    }
    /**
     * @param mixed $primaryColor
     *
     * @return self
     */
    public function setPrimaryColor($primaryColor): self
    {
        $this->initialized['primaryColor'] = true;
        $this->primaryColor = $primaryColor;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getAccentColor()
    {
        return $this->accentColor;
    }
    /**
     * @param mixed $accentColor
     *
     * @return self
     */
    public function setAccentColor($accentColor): self
    {
        $this->initialized['accentColor'] = true;
        $this->accentColor = $accentColor;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getBackgroundColor()
    {
        return $this->backgroundColor;
    }
    /**
     * @param mixed $backgroundColor
     *
     * @return self
     */
    public function setBackgroundColor($backgroundColor): self
    {
        $this->initialized['backgroundColor'] = true;
        $this->backgroundColor = $backgroundColor;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getBackgroundGradient()
    {
        return $this->backgroundGradient;
    }
    /**
     * @param mixed $backgroundGradient
     *
     * @return self
     */
    public function setBackgroundGradient($backgroundGradient): self
    {
        $this->initialized['backgroundGradient'] = true;
        $this->backgroundGradient = $backgroundGradient;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getFooterText()
    {
        return $this->footerText;
    }
    /**
     * @param mixed $footerText
     *
     * @return self
     */
    public function setFooterText($footerText): self
    {
        $this->initialized['footerText'] = true;
        $this->footerText = $footerText;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getCustomCss()
    {
        return $this->customCss;
    }
    /**
     * @param mixed $customCss
     *
     * @return self
     */
    public function setCustomCss($customCss): self
    {
        $this->initialized['customCss'] = true;
        $this->customCss = $customCss;
        return $this;
    }
}
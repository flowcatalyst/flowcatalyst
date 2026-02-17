<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminConfigByAppCodeBySectionByPropertyNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse404
     */
    private $apiAdminConfigAppCodeSectionPropertyGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse404 $apiAdminConfigAppCodeSectionPropertyGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConfigAppCodeSectionPropertyGetResponse404 = $apiAdminConfigAppCodeSectionPropertyGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminConfigAppCodeSectionPropertyGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse404
    {
        return $this->apiAdminConfigAppCodeSectionPropertyGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}
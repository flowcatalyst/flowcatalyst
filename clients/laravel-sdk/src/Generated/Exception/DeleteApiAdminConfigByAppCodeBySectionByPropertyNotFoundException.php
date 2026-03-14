<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminConfigByAppCodeBySectionByPropertyNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyDeleteResponse404
     */
    private $apiAdminConfigAppCodeSectionPropertyDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyDeleteResponse404 $apiAdminConfigAppCodeSectionPropertyDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConfigAppCodeSectionPropertyDeleteResponse404 = $apiAdminConfigAppCodeSectionPropertyDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminConfigAppCodeSectionPropertyDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyDeleteResponse404
    {
        return $this->apiAdminConfigAppCodeSectionPropertyDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}